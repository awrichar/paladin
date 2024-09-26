/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package publictxmgr

import (
	"context"
	"database/sql/driver"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kaleido-io/paladin/core/mocks/componentmocks"
	"github.com/kaleido-io/paladin/core/pkg/ethclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
	"github.com/kaleido-io/paladin/toolkit/pkg/ptxapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newTestOrchestrator(t *testing.T, cbs ...func(mocks *mocksAndTestControl, conf *Config)) (context.Context, *orchestrator, *mocksAndTestControl, func()) {
	ctx, ble, m, done := newTestPublicTxManager(t, false, func(mocks *mocksAndTestControl, conf *Config) {
		mocks.disableManagerStart = true // we don't want the manager running - this gives us a fake nonce manager too
		for _, cb := range cbs {
			cb(mocks, conf)
		}
	})

	signingAddress := tktypes.EthAddress(tktypes.RandBytes(20))
	o := NewOrchestrator(ble, signingAddress, ble.conf)

	return ctx, o, m, done

}

func newInflightTransaction(o *orchestrator, nonce uint64, txMods ...func(tx *DBPublicTxn)) (*InFlightTransactionStageController, *inFlightTransactionState) {
	tx := &DBPublicTxn{
		SignerNonce: fmt.Sprintf("%s:%d", o.signingAddress, 1),
		From:        o.signingAddress,
		Nonce:       nonce,
		Created:     tktypes.TimestampNow(),
	}
	for _, txMod := range txMods {
		txMod(tx)
	}
	mockIT := NewInFlightTransactionStageController(o.pubTxManager, o, tx)
	return mockIT, mockIT.stateManager.(*inFlightTransactionState)
}

func TestNewOrchestratorLoadsSecondTxAndQueuesBalanceCheck(t *testing.T) {

	ctx, o, m, done := newTestOrchestrator(t, func(mocks *mocksAndTestControl, conf *Config) {
		conf.Orchestrator.MaxInFlight = confutil.P(2) // only poll once then we're full
	})
	defer done()

	mockIT, _ := newInflightTransaction(o, 1)

	// Fill first slot with a stage controller
	o.inFlightTxs = []*InFlightTransactionStageController{mockIT}

	// Return the next nonce - will fill up the orchestrator
	m.db.ExpectQuery("SELECT.*public_txn").WillReturnRows(sqlmock.NewRows([]string{"from", "nonce"}).AddRow(
		o.signingAddress, 2,
	))
	// Do not return any submissions for it
	m.db.ExpectQuery("SELECT.*public_submissions").WillReturnRows(sqlmock.NewRows([]string{}))

	addressBalanceChecked := make(chan bool)
	m.ethClient.On("GetBalance", mock.Anything, o.signingAddress, "latest").Return(tktypes.Uint64ToUint256(100), nil).Run(func(args mock.Arguments) {
		close(addressBalanceChecked)
	}).Once()
	_, _ = o.Start(ctx)
	<-addressBalanceChecked
}

func TestNewOrchestratorPollingLoopContextCancelled(t *testing.T) {

	_, o, _, done := newTestOrchestrator(t, func(mocks *mocksAndTestControl, conf *Config) {
		conf.Orchestrator.MaxInFlight = confutil.P(10)
	})
	done()

	o.orchestratorLoopDone = make(chan struct{})
	o.orchestratorLoop()

}

func TestNewOrchestratorPollingContextCancelledWhileRetrying(t *testing.T) {

	ctx, o, m, done := newTestOrchestrator(t, func(mocks *mocksAndTestControl, conf *Config) {
		conf.Orchestrator.MaxInFlight = confutil.P(10)
	})
	defer done()

	o.retry.UTSetMaxAttempts(1) // simulate exit after error
	m.db.ExpectQuery("SELECT.*public_txn").WillReturnError(fmt.Errorf("pop"))

	o.ctxCancel()
	polled, _ := o.pollAndProcess(ctx)
	assert.Equal(t, -1, polled)

}

func TestNewOrchestratorPollingRemoveCompleted(t *testing.T) {

	ctx, o, m, done := newTestOrchestrator(t, func(mocks *mocksAndTestControl, conf *Config) {
		conf.Orchestrator.MaxInFlight = confutil.P(1) // just one inflight, which will trigger poll only after it is done
	})
	defer done()

	// Fill first slot with a stage controller
	mockIT, _ := newInflightTransaction(o, 1)
	mockIT.hasZeroGasPrice = true
	confirmed := InFlightStatusConfirmReceived
	mockIT.newStatus = &confirmed
	o.inFlightTxs = []*InFlightTransactionStageController{mockIT}
	o.state = OrchestratorStateRunning

	// Just keep returning empty rows and we should go idle once we've flushed through the status update above
	m.db.ExpectQuery("SELECT.*public_txn").WillReturnRows(sqlmock.NewRows([]string{}))

	ocDone, _ := o.Start(ctx)

	// It should go idle
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for o.state != OrchestratorStateIdle && !t.Failed() {
		<-ticker.C
	}

	// and that means it should be empty
	assert.Empty(t, o.inFlightTxs)

	// Then we stop it (the manager does this - orchestrators do not stop themselves)
	o.Stop()
	<-ocDone
}

func TestOrchestratorTriggerTopUp(t *testing.T) {

	autoFuelingSourceAddr := *tktypes.RandAddress()
	ctx, o, m, done := newTestOrchestrator(t, func(m *mocksAndTestControl, conf *Config) {
		conf.Orchestrator.MaxInFlight = confutil.P(1) // just one inflight - which we inject in
		conf.BalanceManager.AutoFueling.Source = confutil.P("autofueler")

		m.keyManager.(*componentmocks.KeyManager).On("ResolveKey", mock.Anything, "autofueler", algorithms.ECDSA_SECP256K1_PLAINBYTES).
			Return("", autoFuelingSourceAddr.String(), nil)

	})
	defer done()

	mockIT, txState := newInflightTransaction(o, 1, func(tx *DBPublicTxn) {
		tx.Gas = 100
	})
	txState.ApplyInMemoryUpdates(ctx, &BaseTXUpdates{
		GasPricing: &ptxapi.PublicTxGasPricing{
			GasPrice: tktypes.Int64ToInt256(1000),
		},
	})

	// Fill first slot with a stage controller
	o.inFlightTxs = []*InFlightTransactionStageController{mockIT}
	o.state = OrchestratorStateRunning

	// Mock no auto-fueling TX in flight
	m.db.ExpectQuery("SELECT.*public_txns.*data IS NULL").WillReturnRows(sqlmock.NewRows([]string{}))

	// Then insert of the auto-fueling transaction
	m.db.ExpectBegin()
	m.db.ExpectExec("INSERT.*public_txns").WillReturnResult(driver.ResultNoRows)
	m.db.ExpectCommit()

	// Mock the insufficient balance on the account that's submitting
	m.ethClient.On("GetBalance", mock.Anything, o.signingAddress, "latest").Return(tktypes.Uint64ToUint256(0), nil)

	// Mock the sufficient balance on the auto-fueling source address, and the nonce assignment
	m.ethClient.On("GetBalance", mock.Anything, autoFuelingSourceAddr, "latest").Return(tktypes.Uint64ToUint256(100*1000), nil)
	// Gas estimate for the auto-fueling TX
	m.ethClient.On("EstimateGasNoResolve", mock.Anything, mock.Anything, mock.Anything).
		Return(ethclient.EstimateGasResult{GasLimit: tktypes.HexUint64(10)}, nil)

	_, err := o.Start(ctx)
	require.NoError(t, err)

	var trackedTx *ptxapi.PublicTx
	for trackedTx == nil {
		time.Sleep(10 * time.Millisecond)
		if t.Failed() {
			return
		}
		af := o.balanceManager.(*BalanceManagerWithInMemoryTracking)
		af.addressBalanceChangedMapMux.Lock()
		trackedTx = af.trackedFuelingTransactions[o.signingAddress]
		af.addressBalanceChangedMapMux.Unlock()
	}
}

// func TestOrchestratorHandleConfirmedTransactions(t *testing.T) {
// 	ctx, cancelCtx := context.WithCancel(context.Background())
// 	mockManagedTx0 := &ptxapi.PublicTx{
// 		ID:     uuid.New(),
// 		Status: PubTxStatusPending,
// 		Transaction: &ethsigner.Transaction{
// 			From:  json.RawMessage(testMainSigningAddress),
// 			Nonce: tktypes.Uint64ToUint256(0),
// 		},
// 		Created: tktypes.TimestampNow(),
// 	}
// 	mockManagedTx1 := &ptxapi.PublicTx{
// 		ID:     uuid.New(),
// 		Status: PubTxStatusPending,
// 		Transaction: &ethsigner.Transaction{
// 			From:  json.RawMessage(testMainSigningAddress),
// 			Nonce: tktypes.Uint64ToUint256(1),
// 		},
// 		Created: tktypes.TimestampNow(),
// 	}
// 	mockManagedTx2 := &ptxapi.PublicTx{
// 		ID:     uuid.New(),
// 		Status: PubTxStatusPending,
// 		Transaction: &ethsigner.Transaction{
// 			From:  json.RawMessage(testMainSigningAddress),
// 			Nonce: tktypes.Uint64ToUint256(2),
// 		},
// 		Created: tktypes.TimestampNow(),
// 	}
// 	mockManagedTx3 := &ptxapi.PublicTx{
// 		ID:     uuid.New(),
// 		Status: PubTxStatusPending,
// 		Transaction: &ethsigner.Transaction{
// 			From:  json.RawMessage(testMainSigningAddress),
// 			Nonce: tktypes.Uint64ToUint256(3),
// 		},
// 		Created: tktypes.TimestampNow(),
// 	}
// 	ble, _ := NewTestPublicTxManager(t)
// 	mockBM, mEC, _ := NewTestBalanceManager(ctx, t)
// 	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
// 	mBI := componentmocks.NewBlockIndexer(t)
// 	mTS := componentmocks.NewPublicTransactionStore(t)
// 	mEN := componentmocks.NewPublicTxEventNotifier(t)
// 	mKM := componentmocks.NewKeyManager(t)
// 	ble.Init(ctx, mEC, mKM, mTS, mEN, mBI)

// 	ble.orchestratorConfig.Set(OrchestratorMaxInFlightTransactionsInt, 10)
// 	ble.ctx = ctx
// 	ble.enginePollingInterval = 1 * time.Hour

// 	oc := NewOrchestrator(ble, string(mockManagedTx1.From), ble.orchestratorConfig)

// 	oc.balanceManager = mockBM
// 	mockIT1 := NewInFlightTransactionStageController(ble, oc, mockManagedTx1)
// 	mockIT3 := NewInFlightTransactionStageController(ble, oc, mockManagedTx3)

// 	oc.InFlightTxs = []*InFlightTransactionStageController{
// 		mockIT1,
// 		nil,
// 		mockIT3, // nonce is bigger than the max nonce, so shouldn't be processed
// 	}

// 	assert.Nil(t, oc.confirmedTxNoncePerAddress[testMainSigningAddress])
// 	oc.orchestratorPollingInterval = 1 * time.Hour
// 	err := oc.HandleConfirmedTransactions(ctx, map[string]*blockindexer.IndexedTransaction{
// 		mockManagedTx0.Nonce.BigInt().String(): {}, // already confirmed
// 		mockManagedTx1.Nonce.BigInt().String(): {}, // in flight, add the confirmation event
// 		mockManagedTx2.Nonce.BigInt().String(): {}, // not inflight, so shouldn't be processed
// 	}, big.NewInt(2))
// 	assert.NoError(t, err)
// 	assert.Equal(t, big.NewInt(2), oc.confirmedTxNoncePerAddress[testMainSigningAddress]) //record the max nonce

// 	// cancel context should return with error
// 	cancelCtx()
// 	assert.Regexp(t, "PD010301", oc.HandleConfirmedTransactions(ctx, map[string]*blockindexer.IndexedTransaction{
// 		mockManagedTx0.Nonce.BigInt().String(): {}, // already confirmed
// 		mockManagedTx1.Nonce.BigInt().String(): {}, // in flight, add the confirmation event
// 		mockManagedTx2.Nonce.BigInt().String(): {}, // not inflight, so shouldn't be processed
// 	}, big.NewInt(2)))
// }

// func TestOrchestratorHandleConfirmedTransactionsNoInflightNotHang(t *testing.T) {
// 	ctx := context.Background()

// 	mockManagedTx1 := &ptxapi.PublicTx{
// 		ID:     uuid.New(),
// 		Status: PubTxStatusPending,
// 		Transaction: &ethsigner.Transaction{
// 			From:  json.RawMessage(testMainSigningAddress),
// 			Nonce: tktypes.Uint64ToUint256(1),
// 		},
// 		Created: tktypes.TimestampNow(),
// 	}
// 	ble, _ := NewTestPublicTxManager(t)
// 	mockBM, mEC, _ := NewTestBalanceManager(ctx, t)
// 	ble.gasPriceClient = NewTestFixedPriceGasPriceClient(t)
// 	mBI := componentmocks.NewBlockIndexer(t)
// 	mTS := componentmocks.NewPublicTransactionStore(t)
// 	mEN := componentmocks.NewPublicTxEventNotifier(t)
// 	mKM := componentmocks.NewKeyManager(t)
// 	ble.Init(ctx, mEC, mKM, mTS, mEN, mBI)

// 	ble.orchestratorConfig.Set(OrchestratorMaxInFlightTransactionsInt, 10)
// 	ble.ctx = ctx
// 	ble.enginePollingInterval = 1 * time.Hour

// 	oc := NewOrchestrator(ble, string(mockManagedTx1.From), ble.orchestratorConfig)

// 	oc.balanceManager = mockBM

// 	oc.InFlightTxs = []*InFlightTransactionStageController{}

// 	assert.Nil(t, oc.confirmedTxNoncePerAddress[testMainSigningAddress])
// 	oc.orchestratorPollingInterval = 1 * time.Hour
// 	err := oc.HandleConfirmedTransactions(ctx, map[string]*blockindexer.IndexedTransaction{
// 		mockManagedTx1.Nonce.BigInt().String(): {},
// 	}, big.NewInt(1))
// 	assert.NoError(t, err)
// 	assert.Equal(t, big.NewInt(1), oc.confirmedTxNoncePerAddress[testMainSigningAddress]) //record the max nonce

// }
