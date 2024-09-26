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
	"testing"

	"github.com/kaleido-io/paladin/toolkit/pkg/ptxapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

// import (
// 	"context"
// 	"encoding/json"
// 	"testing"

// 	"github.com/google/uuid"
// 	"github.com/hyperledger/firefly-signer/pkg/ethsigner"
// 	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
// 	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
// 	"github.com/kaleido-io/paladin/toolkit/pkg/confutil"
// 	"github.com/kaleido-io/paladin/toolkit/pkg/ptxapi"
// 	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"

// 	"github.com/stretchr/testify/assert"
// )

const testTransactionData string = "0x7369676e6564206d657373616765"

func NewTestInMemoryTxState(t *testing.T) InMemoryTxStateManager {
	oldTime := tktypes.TimestampNow()
	oldFrom := tktypes.MustEthAddress("0x4e598f6e918321dd47c86e7a077b4ab0e7414846")
	oldTxHash := tktypes.Bytes32(tktypes.RandBytes(32))
	oldTo := tktypes.MustEthAddress("0x6cee73cf4d5b0ac66ce2d1c0617bec4bedd09f39")
	oldNonce := tktypes.HexUint64(1)
	oldGasLimit := tktypes.HexUint64(2000)
	oldValue := tktypes.Uint64ToUint256(200)
	oldGasPrice := tktypes.Uint64ToUint256(10)
	oldErrorMessage := "old message"
	oldTransactionData := tktypes.MustParseHexBytes(testTransactionData)
	testManagedTx := &DBPublicTxn{
		Created: oldTime,
		From:    *oldFrom,
		To:      oldTo,
		Nonce:   oldNonce.Uint64(),
		Gas:     oldGasLimit.Uint64(),
		Value:   oldValue,
		Data:    oldTransactionData,
	}

	imtxs := NewInMemoryTxStateManager(context.Background(), testManagedTx)
	imtxs.ApplyInMemoryUpdates(context.Background(), &BaseTXUpdates{
		NewSubmission: &DBPubTxnSubmission{TransactionHash: oldTxHash},
		GasPricing: &ptxapi.PublicTxGasPricing{
			GasPrice: oldGasPrice,
		},
		FirstSubmit:  &oldTime,
		LastSubmit:   &oldTime,
		ErrorMessage: &oldErrorMessage,
	})
	return imtxs

}

// func TestSettersAndGetters(t *testing.T) {
// 	oldTime := tktypes.TimestampNow()
// 	oldFrom := "0xb3d9cf8e163bbc840195a97e81f8a34e295b8f39"
// 	oldTxHash := tktypes.Bytes32Keccak([]byte("0x00000"))
// 	oldTo := "0x1f9090aae28b8a3dceadf281b0f12828e676c326"
// 	oldNonce := tktypes.Uint64ToUint256(1)
// 	oldGasLimit := tktypes.Uint64ToUint256(2000)
// 	oldValue := tktypes.Uint64ToUint256(200)
// 	oldGasPrice := tktypes.Uint64ToUint256(10)
// 	oldErrorMessage := "old message"
// 	oldTransactionData := ethtypes.MustNewHexBytes0xPrefix(testTransactionData)

// 	testManagedTx := &ptxapi.PublicTx{
// 		ID:              uuid.New(),
// 		Created:         oldTime,
// 		Status:          PubTxStatusPending,
// 		TransactionHash: &oldTxHash,
// 		Transaction: &ethsigner.Transaction{
// 			From:     json.RawMessage(oldFrom),
// 			To:       ethtypes.MustNewAddress(oldTo),
// 			Nonce:    oldNonce,
// 			GasLimit: oldGasLimit,
// 			Value:    oldValue,
// 			GasPrice: oldGasPrice,
// 			Data:     oldTransactionData,
// 		},
// 		SubmittedHashes: []string{
// 			tktypes.Bytes32Keccak([]byte("0x00000")).String(),
// 			tktypes.Bytes32Keccak([]byte("0x00001")).String(),
// 			tktypes.Bytes32Keccak([]byte("0x00002")).String(),
// 		},
// 		FirstSubmit:  &oldTime,
// 		LastSubmit:   &oldTime,
// 		ErrorMessage: &oldErrorMessage,
// 	}

// 	inMemoryTxState := NewInMemoryTxStateManager(context.Background(), testManagedTx)

// 	inMemoryTx := inMemoryTxState.GetTx()

// 	assert.Equal(t, testManagedTx.ID, inMemoryTxState.GetSignerNonce())

// 	assert.Equal(t, oldTime, *inMemoryTxState.GetCreatedTime())
// 	assert.Nil(t, inMemoryTxState.GetConfirmedTransaction())
// 	assert.Equal(t, oldTxHash, *inMemoryTxState.GetTransactionHash())
// 	assert.Equal(t, oldNonce.BigInt(), inMemoryTxState.GetNonce())
// 	assert.Equal(t, oldFrom, inMemoryTxState.GetFrom())
// 	assert.Equal(t, testManagedTx.Status, inMemoryTxState.GetStatus())
// 	assert.Equal(t, oldGasPrice.BigInt(), inMemoryTxState.GetGasPriceObject().GasPrice)
// 	assert.Equal(t, oldTime, *inMemoryTxState.GetFirstSubmit())
// 	assert.Equal(t, []string{
// 		tktypes.Bytes32Keccak([]byte("0x00000")).String(),
// 		tktypes.Bytes32Keccak([]byte("0x00001")).String(),
// 		tktypes.Bytes32Keccak([]byte("0x00002")).String(),
// 	}, inMemoryTxState.GetSubmittedHashes())
// 	assert.Equal(t, testManagedTx, inMemoryTxState.GetTx())
// 	assert.Equal(t, oldGasLimit.BigInt(), inMemoryTxState.GetGasLimit())
// 	assert.False(t, inMemoryTxState.IsComplete())

// 	// add indexed to the pending transaction and mark it as complete
// 	testConfirmedTx := &blockindexer.IndexedTransaction{
// 		BlockNumber:      int64(1233),
// 		TransactionIndex: int64(23),
// 		Hash:             tktypes.Bytes32Keccak([]byte("test")),
// 		Result:           blockindexer.TXResult_SUCCESS.Enum(),
// 	}

// 	successStatus := PubTxStatusSucceeded
// 	newTime := confutil.P(tktypes.TimestampNow())
// 	newTxHash := tktypes.Bytes32Keccak([]byte("0x000031"))
// 	newGasLimit := tktypes.Uint64ToUint256(111)
// 	newGasPrice := tktypes.Uint64ToUint256(111)
// 	newErrorMessage := "new message"

// 	inMemoryTxState.ApplyTxUpdates(context.Background(), &BaseTXUpdates{
// 		Status:          &successStatus,
// 		GasPrice:        newGasPrice,
// 		TransactionHash: &newTxHash,
// 		NewSubmittedHashes: []string{
// 			tktypes.Bytes32Keccak([]byte("0x00000")).String(),
// 			tktypes.Bytes32Keccak([]byte("0x00001")).String(),
// 			tktypes.Bytes32Keccak([]byte("0x00002")).String(),
// 			tktypes.Bytes32Keccak([]byte("0x00003")).String(),
// 		},
// 		FirstSubmit:  newTime,
// 		LastSubmit:   newTime,
// 		ErrorMessage: &newErrorMessage,
// 		GasLimit:     newGasLimit,
// 	})

// 	assert.Equal(t, testManagedTx.ID, inMemoryTxState.GetSignerNonce())

// 	assert.Equal(t, oldTime, *inMemoryTxState.GetCreatedTime())
// 	assert.Equal(t, newTime, inMemoryTxState.GetLastSubmitTime())
// 	inMemoryTxState.SetConfirmedTransaction(context.Background(), testConfirmedTx)
// 	assert.Equal(t, testConfirmedTx, inMemoryTxState.GetConfirmedTransaction())
// 	assert.Equal(t, newTxHash, *inMemoryTxState.GetTransactionHash())
// 	assert.Equal(t, successStatus, inMemoryTxState.GetStatus())
// 	assert.Equal(t, newGasPrice.BigInt(), inMemoryTxState.GetGasPriceObject().GasPrice)
// 	assert.Nil(t, inMemoryTxState.GetGasPriceObject().MaxFeePerGas)
// 	assert.Nil(t, inMemoryTxState.GetGasPriceObject().MaxPriorityFeePerGas)
// 	assert.Equal(t, newTime, inMemoryTxState.GetFirstSubmit())
// 	assert.Equal(t, []string{
// 		tktypes.Bytes32Keccak([]byte("0x00000")).String(),
// 		tktypes.Bytes32Keccak([]byte("0x00001")).String(),
// 		tktypes.Bytes32Keccak([]byte("0x00002")).String(),
// 		tktypes.Bytes32Keccak([]byte("0x00003")).String(),
// 	}, inMemoryTxState.GetSubmittedHashes())
// 	assert.Equal(t, testManagedTx, inMemoryTxState.GetTx())
// 	assert.Equal(t, newGasLimit.BigInt(), inMemoryTxState.GetGasLimit())
// 	assert.True(t, inMemoryTxState.IsComplete())

// 	// check immutable fields
// 	assert.Equal(t, oldNonce.BigInt(), inMemoryTxState.GetNonce())
// 	assert.Equal(t, oldFrom, inMemoryTxState.GetFrom())
// 	assert.Equal(t, oldValue, inMemoryTx.Value)
// 	assert.Equal(t, oldTransactionData, inMemoryTx.Data)

// 	maxPriorityFeePerGas := tktypes.Uint64ToUint256(2)
// 	maxFeePerGas := tktypes.Uint64ToUint256(123)

// 	// test switch gas price format
// 	inMemoryTxState.ApplyTxUpdates(context.Background(), &BaseTXUpdates{
// 		MaxPriorityFeePerGas: maxPriorityFeePerGas,
// 		MaxFeePerGas:         maxFeePerGas,
// 	})

// 	assert.Nil(t, inMemoryTxState.GetGasPriceObject().GasPrice)
// 	assert.Equal(t, maxFeePerGas.BigInt(), inMemoryTxState.GetGasPriceObject().MaxFeePerGas)
// 	assert.Equal(t, maxPriorityFeePerGas.BigInt(), inMemoryTxState.GetGasPriceObject().MaxPriorityFeePerGas)

// 	// test switch back and prefer legacy gas price

// 	maxPF := tktypes.Uint64ToUint256(3)
// 	maxF := tktypes.Uint64ToUint256(234)
// 	maxP := tktypes.Uint64ToUint256(10000)
// 	inMemoryTxState.ApplyTxUpdates(context.Background(), &BaseTXUpdates{
// 		MaxPriorityFeePerGas: maxPF,
// 		MaxFeePerGas:         maxF,
// 		GasPrice:             maxP,
// 	})

// 	assert.Equal(t, maxP.BigInt(), inMemoryTxState.GetGasPriceObject().GasPrice)
// 	assert.Nil(t, inMemoryTxState.GetGasPriceObject().MaxFeePerGas)
// 	assert.Nil(t, inMemoryTxState.GetGasPriceObject().MaxPriorityFeePerGas)
// }
