// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package statemgr

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/filters"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fakeCoinABI = `{
	"type": "tuple",
	"internalType": "struct FakeCoin",
	"components": [
		{
			"name": "salt",
			"type": "bytes32"
		},
		{
			"name": "owner",
			"type": "address",
			"indexed": true
		},
		{
			"name": "amount",
			"type": "uint256",
			"indexed": true
		}
	]
}`

type FakeCoin struct {
	Amount ethtypes.HexInteger       `json:"amount"`
	Salt   ethtypes.HexBytes0xPrefix `json:"salt"`
}

func parseFakeCoin(t *testing.T, s *components.State) *FakeCoin {
	var c FakeCoin
	err := json.Unmarshal(s.Data, &c)
	require.NoError(t, err)
	return &c
}

func TestStateFlushAsync(t *testing.T) {

	_, ss, done := newDBTestStateManager(t)
	defer done()

	contractAddress := tktypes.RandAddress()
	flushed := make(chan bool)
	err := ss.RunInDomainContext("domain1", *contractAddress, func(ctx context.Context, dsi components.DomainStateInterface) error {
		return dsi.Flush(func(ctx context.Context, dsi components.DomainStateInterface) error {
			flushed <- true
			return nil
		})
	})
	require.NoError(t, err)

	select {
	case <-flushed:
	case <-time.After(5 * time.Second):
		assert.Fail(t, "timed out")
	}

}

func TestUpsertSchemaEmptyList(t *testing.T) {

	_, ss, done := newDBTestStateManager(t)
	defer done()

	schemas, err := ss.EnsureABISchemas(context.Background(), "domain1", []*abi.Parameter{})
	require.NoError(t, err)
	require.Len(t, schemas, 0)

}

func TestUpsertSchemaAndStates(t *testing.T) {

	_, ss, done := newDBTestStateManager(t)
	defer done()

	schemas, err := ss.EnsureABISchemas(context.Background(), "domain1", []*abi.Parameter{testABIParam(t, fakeCoinABI)})
	require.NoError(t, err)
	require.Len(t, schemas, 1)
	schemaID := schemas[0].IDString()
	fakeHash := tktypes.HexBytes(tktypes.RandBytes(32))

	contractAddress := tktypes.RandAddress()
	err = ss.RunInDomainContext("domain1", *contractAddress, func(ctx context.Context, dsi components.DomainStateInterface) error {
		states, err := dsi.UpsertStates(nil, []*components.StateUpsert{
			{
				SchemaID: schemaID,
				Data:     tktypes.RawJSON(fmt.Sprintf(`{"amount": 100, "owner": "0x1eDfD974fE6828dE81a1a762df680111870B7cDD", "salt": "%s"}`, tktypes.RandHex(32))),
			},
			{
				ID:       fakeHash,
				SchemaID: schemaID,
				Data:     tktypes.RawJSON(fmt.Sprintf(`{"amount": 100, "owner": "0x1eDfD974fE6828dE81a1a762df680111870B7cDD", "salt": "%s"}`, tktypes.RandHex(32))),
			},
		})
		require.NoError(t, err)
		require.Len(t, states, 2)
		assert.NotEmpty(t, states[0].ID)
		assert.Equal(t, fakeHash, states[1].ID)
		return nil
	})
	require.NoError(t, err)

}

func TestStateContextMintSpendMint(t *testing.T) {

	_, ss, done := newDBTestStateManager(t)
	defer done()

	transactionID := uuid.New()
	var schemaID string

	// Pop in our widget ABI
	schemas, err := ss.EnsureABISchemas(context.Background(), "domain1", []*abi.Parameter{testABIParam(t, fakeCoinABI)})
	require.NoError(t, err)
	assert.Len(t, schemas, 1)
	schemaID = schemas[0].IDString()

	contractAddress := tktypes.RandAddress()
	err = ss.RunInDomainContextFlush("domain1", *contractAddress, func(ctx context.Context, dsi components.DomainStateInterface) error {

		// Store some states
		tx1states, err := dsi.UpsertStates(&transactionID, []*components.StateUpsert{
			{SchemaID: schemaID, Data: tktypes.RawJSON(fmt.Sprintf(`{"amount": 100, "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, tktypes.RandHex(32))), Creating: true},
			{SchemaID: schemaID, Data: tktypes.RawJSON(fmt.Sprintf(`{"amount": 10,  "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, tktypes.RandHex(32))), Creating: true},
			{SchemaID: schemaID, Data: tktypes.RawJSON(fmt.Sprintf(`{"amount": 75,  "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, tktypes.RandHex(32))), Creating: true},
		})
		require.NoError(t, err)
		assert.Len(t, tx1states, 3)

		// Mark an in-memory read - doesn't affect it's availability, but will be locked to that transaction
		err = dsi.MarkStatesRead(transactionID, []string{tx1states[0].ID.String()})
		require.NoError(t, err)

		// We can't arbitrarily move it to another transaction (would need to reset the first transaction)
		err = dsi.MarkStatesRead(uuid.New(), []string{tx1states[0].ID.String()})
		assert.Regexp(t, "PD010118", err)
		err = dsi.MarkStatesSpending(uuid.New(), []string{tx1states[0].ID.String()})
		assert.Regexp(t, "PD010118", err)

		// Query the states, and notice we find the ones that are still in the process of creating
		// even though they've not yet been written to the DB
		states, err := dsi.FindAvailableStates(schemaID, toQuery(t, `{
			"sort": [ "amount" ]
		}`))
		require.NoError(t, err)
		assert.Len(t, states, 3)

		// The values should be sorted according to the requested order
		assert.Equal(t, int64(10), parseFakeCoin(t, states[0]).Amount.Int64())
		assert.Equal(t, int64(75), parseFakeCoin(t, states[1]).Amount.Int64())
		assert.Equal(t, int64(100), parseFakeCoin(t, states[2]).Amount.Int64())
		assert.True(t, states[0].Locked.Creating)                    // should be marked creating
		assert.Equal(t, transactionID, states[0].Locked.Transaction) // for the transaction we specified

		// Simulate a transaction where we spend two states, and create 2 new ones
		err = dsi.MarkStatesSpending(transactionID, []string{
			states[0].ID.String(), // 10 +
			states[1].ID.String(), // 75
		})
		require.NoError(t, err)

		// Do a quick check on upsert semantics with un-flushed updates, to make sure the unflushed list doesn't dup
		tx2Salts := []string{tktypes.RandHex(32), tktypes.RandHex(32)}
		for dup := 0; dup < 2; dup++ {
			tx2states, err := dsi.UpsertStates(&transactionID, []*components.StateUpsert{
				{SchemaID: schemaID, Data: tktypes.RawJSON(fmt.Sprintf(`{"amount": 35, "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, tx2Salts[0])), Creating: true},
				{SchemaID: schemaID, Data: tktypes.RawJSON(fmt.Sprintf(`{"amount": 50, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`, tx2Salts[1])), Creating: true},
			})
			require.NoError(t, err)
			assert.Len(t, tx2states, 2)
			assert.Equal(t, len(dsi.(*domainContext).unFlushed.states), 5)
			assert.Equal(t, len(dsi.(*domainContext).unFlushed.stateLocks), 5)
		}

		// Query the states on the first address
		states, err = dsi.FindAvailableStates(schemaID, toQuery(t, `{
			"sort": [ "-amount" ],
			"eq": [{"field": "owner", "value": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180"}]
		}`))
		require.NoError(t, err)
		assert.Len(t, states, 2)
		assert.Equal(t, int64(100), parseFakeCoin(t, states[0]).Amount.Int64())
		assert.Equal(t, int64(35), parseFakeCoin(t, states[1]).Amount.Int64())

		// Query the states on the other address
		states, err = dsi.FindAvailableStates(schemaID, toQuery(t, `{
					"sort": [ "-amount" ],
					"eq": [{"field": "owner", "value": "0x615dD09124271D8008225054d85Ffe720E7a447A"}]
				}`))
		require.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, int64(50), parseFakeCoin(t, states[0]).Amount.Int64())

		// Flush the states to the database
		return nil
	})
	require.NoError(t, err)

	err = ss.RunInDomainContextFlush("domain1", *contractAddress, func(ctx context.Context, dsi components.DomainStateInterface) error {
		// Check the DB persisted state is what we expect
		states, err := dsi.FindAvailableStates(schemaID, toQuery(t, `{
			"sort": [ "owner", "amount" ]
		}`))
		require.NoError(t, err)
		assert.Len(t, states, 3)
		assert.Equal(t, int64(50), parseFakeCoin(t, states[0]).Amount.Int64())
		assert.Equal(t, int64(35), parseFakeCoin(t, states[1]).Amount.Int64())
		assert.Equal(t, int64(100), parseFakeCoin(t, states[2]).Amount.Int64())

		// Mark a persisted one read - doesn't affect it's availability, but will be locked to that transaction
		err = dsi.MarkStatesRead(transactionID, []string{
			states[1].ID.String(),
		})
		require.NoError(t, err)

		// Write another transaction that splits a coin to two
		err = dsi.MarkStatesSpending(transactionID, []string{
			states[0].ID.String(), // 50
		})
		require.NoError(t, err)
		tx3states, err := dsi.UpsertStates(&transactionID, []*components.StateUpsert{
			{SchemaID: schemaID, Data: tktypes.RawJSON(fmt.Sprintf(`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`, tktypes.RandHex(32))), Creating: true},
			{SchemaID: schemaID, Data: tktypes.RawJSON(fmt.Sprintf(`{"amount": 30, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`, tktypes.RandHex(32))), Creating: true},
		})
		require.NoError(t, err)
		assert.Len(t, tx3states, 2)

		// Now check that we merge the DB and in-memory state
		states, err = dsi.FindAvailableStates(schemaID, toQuery(t, `{
			"sort": [ "owner", "amount" ]
		}`))
		require.NoError(t, err)
		assert.Len(t, states, 4)
		assert.Equal(t, int64(20), parseFakeCoin(t, states[0]).Amount.Int64())
		assert.Equal(t, int64(30), parseFakeCoin(t, states[1]).Amount.Int64())
		assert.Equal(t, int64(35), parseFakeCoin(t, states[2]).Amount.Int64())
		assert.Equal(t, int64(100), parseFakeCoin(t, states[3]).Amount.Int64())

		// Check the limit works too across this
		states, err = dsi.FindAvailableStates(schemaID, toQuery(t, `{
			"limit": 1,
			"sort": [ "owner", "amount" ]
		}`))
		require.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, int64(20), parseFakeCoin(t, states[0]).Amount.Int64())

		// Mark a state confirmed
		confirmState := states[0].ID.String() // 20
		err = dsi.MarkStatesConfirmed(transactionID, []string{confirmState})
		require.NoError(t, err)

		// Can't confirm again from a different transaction (but can from the same transaction)
		err = dsi.MarkStatesConfirmed(uuid.New(), []string{confirmState})
		require.ErrorContains(t, err, "PD010121")
		err = dsi.MarkStatesConfirmed(transactionID, []string{confirmState})
		require.NoError(t, err)

		// Mark a state spent
		spendState := states[0].ID.String() // 20
		err = dsi.MarkStatesSpent(transactionID, []string{spendState})
		require.NoError(t, err)

		// Check the remaining states
		states, err = dsi.FindAvailableStates(schemaID, toQuery(t, `{
			"sort": [ "owner", "amount" ]
		}`))
		require.NoError(t, err)
		assert.Len(t, states, 3)
		assert.Equal(t, int64(30), parseFakeCoin(t, states[0]).Amount.Int64())
		assert.Equal(t, int64(35), parseFakeCoin(t, states[1]).Amount.Int64())
		assert.Equal(t, int64(100), parseFakeCoin(t, states[2]).Amount.Int64())

		// Can't spend again from a different transaction (but can from the same transaction)
		err = dsi.MarkStatesSpent(uuid.New(), []string{spendState})
		require.ErrorContains(t, err, "PD010120")
		err = dsi.MarkStatesSpent(transactionID, []string{spendState})
		require.NoError(t, err)

		// Reset the transaction - this will clear the in-memory state,
		// and remove the locks from the DB. It will not remove the states
		// themselves
		err = dsi.ResetTransaction(transactionID)
		require.NoError(t, err)

		// None of the states will be returned to available after the flush
		// - but before then the DB ones will be
		return nil
	})
	require.NoError(t, err)

	err = ss.RunInDomainContextFlush("domain1", *contractAddress, func(ctx context.Context, dsi components.DomainStateInterface) error {

		// Confirm
		states, err := dsi.FindAvailableStates(schemaID, toQuery(t, `{}`))
		require.NoError(t, err)
		assert.Empty(t, states)

		return nil
	})
	require.NoError(t, err)

}

func TestStateContextMintSpendWithNullifier(t *testing.T) {

	_, ss, done := newDBTestStateManager(t)
	defer done()

	transactionID := uuid.New()
	var schemaID string

	schemas, err := ss.EnsureABISchemas(context.Background(), "domain1", []*abi.Parameter{testABIParam(t, fakeCoinABI)})
	require.NoError(t, err)
	assert.Len(t, schemas, 1)
	schemaID = schemas[0].IDString()
	stateID1 := tktypes.HexBytes(tktypes.RandBytes(32))
	stateID2 := tktypes.HexBytes(tktypes.RandBytes(32))
	nullifier1 := tktypes.HexBytes(tktypes.RandBytes(32))
	nullifier2 := tktypes.HexBytes(tktypes.RandBytes(32))
	data1 := tktypes.RawJSON(fmt.Sprintf(`{"amount": 100, "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, tktypes.RandHex(32)))
	data2 := tktypes.RawJSON(fmt.Sprintf(`{"amount": 10,  "owner": "0xf7b1c69F5690993F2C8ecE56cc89D42b1e737180", "salt": "%s"}`, tktypes.RandHex(32)))

	contractAddress := tktypes.RandAddress()
	err = ss.RunInDomainContextFlush("domain1", *contractAddress, func(ctx context.Context, dsi components.DomainStateInterface) error {

		// Start with 2 states
		tx1states, err := dsi.UpsertStates(&transactionID, []*components.StateUpsert{
			{ID: stateID1, SchemaID: schemaID, Data: data1, Creating: true},
			{ID: stateID2, SchemaID: schemaID, Data: data2, Creating: true},
		})
		require.NoError(t, err)
		assert.Len(t, tx1states, 2)

		states, err := dsi.FindAvailableStates(schemaID, toQuery(t, `{}`))
		require.NoError(t, err)
		assert.Len(t, states, 2)
		states, err = dsi.FindAvailableNullifiers(schemaID, toQuery(t, `{}`))
		require.NoError(t, err)
		assert.Len(t, states, 0)

		// Attach a nullifier to the first state
		err = dsi.UpsertNullifiers([]*components.StateNullifier{
			{State: stateID1, Nullifier: nullifier1},
		})
		require.NoError(t, err)

		states, err = dsi.FindAvailableNullifiers(schemaID, toQuery(t, `{}`))
		require.NoError(t, err)
		require.Len(t, states, 1)
		require.NotNil(t, states[0].Nullifier)
		assert.Equal(t, nullifier1, states[0].Nullifier.Nullifier)

		// Flush the states to the database
		return nil
	})
	require.NoError(t, err)

	err = ss.RunInDomainContextFlush("domain1", *contractAddress, func(ctx context.Context, dsi components.DomainStateInterface) error {

		// Confirm still 2 states and 1 nullifier
		states, err := dsi.FindAvailableStates(schemaID, toQuery(t, `{}`))
		require.NoError(t, err)
		assert.Len(t, states, 2)
		states, err = dsi.FindAvailableNullifiers(schemaID, toQuery(t, `{}`))
		require.NoError(t, err)
		assert.Len(t, states, 1)
		require.NotNil(t, states[0].Nullifier)
		assert.Equal(t, nullifier1, states[0].Nullifier.Nullifier)

		// Mark both states confirmed
		err = dsi.MarkStatesConfirmed(transactionID, []string{stateID1.String(), stateID2.String()})
		require.NoError(t, err)

		// Mark the first state as "spending"
		_, err = dsi.UpsertStates(&transactionID, []*components.StateUpsert{
			{ID: stateID1, SchemaID: schemaID, Data: data1, Spending: true},
		})
		assert.NoError(t, err)

		// Confirm no more nullifiers available
		states, err = dsi.FindAvailableNullifiers(schemaID, toQuery(t, `{}`))
		require.NoError(t, err)
		assert.Len(t, states, 0)

		// Reset transaction to unlock
		err = dsi.ResetTransaction(transactionID)
		assert.NoError(t, err)
		states, err = dsi.FindAvailableNullifiers(schemaID, toQuery(t, `{}`))
		require.NoError(t, err)
		assert.Len(t, states, 1)

		// Spend the nullifier
		err = dsi.MarkStatesSpent(transactionID, []string{nullifier1.String()})
		assert.NoError(t, err)

		// Confirm no more nullifiers available
		states, err = dsi.FindAvailableNullifiers(schemaID, toQuery(t, `{}`))
		require.NoError(t, err)
		assert.Len(t, states, 0)

		// Flush the states to the database
		return nil
	})
	require.NoError(t, err)

	err = ss.RunInDomainContextFlush("domain1", *contractAddress, func(ctx context.Context, dsi components.DomainStateInterface) error {

		states, err := dsi.FindAvailableNullifiers(schemaID, toQuery(t, `{}`))
		require.NoError(t, err)
		assert.Len(t, states, 0)

		// Attach a nullifier to the second state
		err = dsi.UpsertNullifiers([]*components.StateNullifier{
			{State: stateID2, Nullifier: nullifier2},
		})
		require.NoError(t, err)

		states, err = dsi.FindAvailableNullifiers(schemaID, toQuery(t, `{}`))
		require.NoError(t, err)
		require.Len(t, states, 1)
		require.NotNil(t, states[0].Nullifier)
		assert.Equal(t, nullifier2, states[0].Nullifier.Nullifier)

		return nil
	})
	require.NoError(t, err)

}

func TestDSILatch(t *testing.T) {

	_, ss, done := newDBTestStateManager(t)

	contractAddress := tktypes.RandAddress()
	dsi := ss.getDomainContext("domain1", *contractAddress)
	err := dsi.takeLatch()
	require.NoError(t, err)

	done()
	err = dsi.run(func(ctx context.Context, dsi components.DomainStateInterface) error { return nil })
	assert.Regexp(t, "PD010301", err)

}

func TestDSIBadSchema(t *testing.T) {

	_, ss, _, done := newDBMockStateManager(t)
	defer done()

	_, err := ss.EnsureABISchemas(context.Background(), "domain1", []*abi.Parameter{{}})
	assert.Regexp(t, "PD010114", err)

}

func TestDSIFlushErrorCapture(t *testing.T) {

	_, ss, done := newDBTestStateManager(t)
	defer done()

	fakeFlushError := func(dc *domainContext) {
		dc.flushing = &writeOperation{}
		dc.flushResult = make(chan error, 1)
		dc.flushResult <- fmt.Errorf("pop")
	}

	schemas, err := ss.EnsureABISchemas(context.Background(), "domain1", []*abi.Parameter{testABIParam(t, fakeCoinABI)})
	require.NoError(t, err)

	contractAddress := tktypes.RandAddress()
	err = ss.RunInDomainContextFlush("domain1", *contractAddress, func(ctx context.Context, dsi components.DomainStateInterface) error {

		dc := dsi.(*domainContext)

		fakeFlushError(dc)
		_, err = dsi.FindAvailableStates("", nil)
		assert.Regexp(t, "pop", err)

		fakeFlushError(dc)
		_, err = dsi.FindAvailableNullifiers("", nil)
		assert.Regexp(t, "pop", err)

		fakeFlushError(dc)
		schema, err := ss.getSchemaByID(ctx, "domain1", tktypes.MustParseBytes32(schemas[0].IDString()), true)
		require.NoError(t, err)
		_, err = dc.mergedUnFlushed(schema, nil, nil, false)
		assert.Regexp(t, "pop", err)

		fakeFlushError(dc)
		_, err = dsi.UpsertStates(nil, nil)
		assert.Regexp(t, "pop", err)

		fakeFlushError(dc)
		err = dsi.UpsertNullifiers(nil)
		assert.Regexp(t, "pop", err)

		fakeFlushError(dc)
		err = dsi.MarkStatesRead(uuid.New(), nil)
		assert.Regexp(t, "pop", err)

		fakeFlushError(dc)
		err = dsi.MarkStatesSpending(uuid.New(), nil)
		assert.Regexp(t, "pop", err)

		fakeFlushError(dc)
		err = dsi.MarkStatesSpent(uuid.New(), nil)
		assert.Regexp(t, "pop", err)

		fakeFlushError(dc)
		err = dsi.MarkStatesConfirmed(uuid.New(), nil)
		assert.Regexp(t, "pop", err)

		fakeFlushError(dc)
		err = dsi.ResetTransaction(uuid.New())
		assert.Regexp(t, "pop", err)

		fakeFlushError(dc)
		err = dsi.Flush()
		assert.Regexp(t, "pop", err)

		return nil

	})
	require.NoError(t, err)

}

func TestDSIMergedUnFlushedWhileFlushing(t *testing.T) {

	ctx, ss, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)

	contractAddress := tktypes.RandAddress()
	dc := ss.getDomainContext("domain1", *contractAddress)

	s1, err := schema.ProcessState(ctx, *contractAddress, tktypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		tktypes.RandHex(32))), nil)
	require.NoError(t, err)
	s1.Locked = &components.StateLock{State: s1.ID, Transaction: uuid.New(), Creating: true}

	dc.flushing = &writeOperation{
		states: []*components.StateWithLabels{s1},
		stateLocks: []*components.StateLock{
			s1.Locked,
			{State: []byte("another"), Spending: true},
		},
	}

	spending, _, _, err := dc.getUnFlushedStates()
	require.NoError(t, err)
	assert.Len(t, spending, 1)

	states, err := dc.mergedUnFlushed(schema, []*components.State{}, &query.QueryJSON{
		Sort: []string{".created"},
	}, false)
	require.NoError(t, err)
	assert.Len(t, states, 1)

}

func TestDSIMergedUnFlushedSpend(t *testing.T) {

	ctx, ss, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)

	contractAddress := tktypes.RandAddress()
	dc := ss.getDomainContext("domain1", *contractAddress)

	s1, err := schema.ProcessState(ctx, *contractAddress, tktypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		tktypes.RandHex(32))), nil)
	require.NoError(t, err)
	s1.Locked = &components.StateLock{State: s1.ID, Transaction: uuid.New(), Creating: true}

	dc.flushing = &writeOperation{
		states: []*components.StateWithLabels{s1},
		stateSpends: []*components.StateSpend{
			{State: s1.ID[:]},
		},
	}
	dc.unFlushed = &writeOperation{
		stateSpends: []*components.StateSpend{
			{State: tktypes.RandBytes(32)},
		},
	}

	_, spent, _, err := dc.getUnFlushedStates()
	require.NoError(t, err)
	assert.Len(t, spent, 2)

	states, err := dc.mergedUnFlushed(schema, []*components.State{}, &query.QueryJSON{}, false)
	require.NoError(t, err)
	assert.Len(t, states, 0)

}

func TestDSIMergedUnFlushedWhileFlushingDedup(t *testing.T) {

	ctx, ss, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)

	contractAddress := tktypes.RandAddress()
	dc := ss.getDomainContext("domain1", *contractAddress)

	s1, err := schema.ProcessState(ctx, *contractAddress, tktypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		tktypes.RandHex(32))), nil)
	require.NoError(t, err)
	s1.Locked = &components.StateLock{State: s1.ID, Transaction: uuid.New(), Creating: true}

	dc.flushing = &writeOperation{
		states: []*components.StateWithLabels{s1},
		stateLocks: []*components.StateLock{
			s1.Locked,
			{State: []byte("another"), Spending: true},
		},
	}

	spending, _, _, err := dc.getUnFlushedStates()
	require.NoError(t, err)
	assert.Len(t, spending, 1)

	dc.stateLock.Lock()
	inTheFlush := dc.flushing.states[0]
	dc.stateLock.Unlock()

	states, err := dc.mergedUnFlushed(schema, []*components.State{
		inTheFlush.State,
	}, &query.QueryJSON{
		Sort: []string{".created"},
	}, false)
	require.NoError(t, err)
	assert.Len(t, states, 1)

}

func TestDSIMergedUnFlushedEvalError(t *testing.T) {

	ctx, ss, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)

	contractAddress := tktypes.RandAddress()
	dc := ss.getDomainContext("domain1", *contractAddress)

	s1, err := schema.ProcessState(ctx, *contractAddress, tktypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		tktypes.RandHex(32))), nil)
	require.NoError(t, err)

	dc.flushing = &writeOperation{
		states: []*components.StateWithLabels{s1},
	}

	_, err = dc.mergedUnFlushed(schema, []*components.State{}, toQuery(t,
		`{"eq": [{ "field": "wrong", "value": "any" }]}`,
	), false)
	assert.Regexp(t, "PD010700", err)

}

func TestDSIMergedInMemoryMatchesRecoverLabelsFail(t *testing.T) {

	ctx, ss, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)

	contractAddress := tktypes.RandAddress()
	dc := ss.getDomainContext("domain1", *contractAddress)

	s1, err := schema.ProcessState(ctx, *contractAddress, tktypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		tktypes.RandHex(32))), nil)
	require.NoError(t, err)
	s1.Data = tktypes.RawJSON(`! wrong `)

	dc.flushing = &writeOperation{
		states: []*components.StateWithLabels{s1},
	}

	_, err = dc.mergeInMemoryMatches(schema, []*components.State{
		s1.State,
	}, []*components.StateWithLabels{}, nil)
	assert.Regexp(t, "PD010116", err)

}

func TestDSIMergedInMemoryMatchesSortFail(t *testing.T) {

	ctx, ss, _, done := newDBMockStateManager(t)
	defer done()

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)

	contractAddress := tktypes.RandAddress()
	dc := ss.getDomainContext("domain1", *contractAddress)

	s1, err := schema.ProcessState(ctx, *contractAddress, tktypes.RawJSON(fmt.Sprintf(
		`{"amount": 20, "owner": "0x615dD09124271D8008225054d85Ffe720E7a447A", "salt": "%s"}`,
		tktypes.RandHex(32))), nil)
	require.NoError(t, err)

	dc.flushing = &writeOperation{
		states: []*components.StateWithLabels{s1},
	}

	_, err = dc.mergeInMemoryMatches(schema, []*components.State{
		s1.State,
	}, []*components.StateWithLabels{}, toQuery(t,
		`{"sort": ["wrong"]}`,
	))
	assert.Regexp(t, "PD010700", err)
}

func TestDSIFindBadQueryAndInsert(t *testing.T) {

	_, ss, done := newDBTestStateManager(t)
	defer done()

	schemas, err := ss.EnsureABISchemas(context.Background(), "domain1", []*abi.Parameter{testABIParam(t, fakeCoinABI)})
	require.NoError(t, err)
	schemaID := schemas[0].IDString()
	assert.Equal(t, "type=FakeCoin(bytes32 salt,address owner,uint256 amount),labels=[owner,amount]", schemas[0].Signature())

	contractAddress := tktypes.RandAddress()
	err = ss.RunInDomainContextFlush("domain1", *contractAddress, func(ctx context.Context, dsi components.DomainStateInterface) error {
		_, err = dsi.FindAvailableStates(schemaID, toQuery(t,
			`{"sort":["wrong"]}`))
		assert.Regexp(t, "PD010700", err)

		_, err = dsi.FindAvailableNullifiers(schemaID, toQuery(t,
			`{"sort":["wrong"]}`))
		assert.Regexp(t, "PD010700", err)

		_, err = dsi.UpsertStates(nil, []*components.StateUpsert{
			{SchemaID: schemaID, Data: tktypes.RawJSON(`"wrong"`)},
		})
		assert.Regexp(t, "FF22038", err)

		return nil
	})
	require.NoError(t, err)

}

func TestDSIBadIDs(t *testing.T) {

	_, ss, _, done := newDBMockStateManager(t)
	defer done()

	contractAddress := tktypes.RandAddress()
	_ = ss.RunInDomainContext("domain1", *contractAddress, func(ctx context.Context, dsi components.DomainStateInterface) error {

		_, err := dsi.UpsertStates(nil, []*components.StateUpsert{
			{SchemaID: "wrong"},
		})
		assert.Regexp(t, "PD020007", err)

		err = dsi.MarkStatesRead(uuid.New(), []string{"wrong"})
		assert.Regexp(t, "PD020007", err)

		err = dsi.MarkStatesSpending(uuid.New(), []string{"wrong"})
		assert.Regexp(t, "PD020007", err)

		err = dsi.MarkStatesSpent(uuid.New(), []string{"wrong"})
		assert.Regexp(t, "PD020007", err)

		err = dsi.MarkStatesConfirmed(uuid.New(), []string{"wrong"})
		assert.Regexp(t, "PD020007", err)

		return nil
	})

}

func TestDSIResetWithMixed(t *testing.T) {

	_, ss, _, done := newDBMockStateManager(t)
	defer done()

	contractAddress := tktypes.RandAddress()
	dc := ss.getDomainContext("domain1", *contractAddress)

	state1 := tktypes.HexBytes("state1")
	transactionID1 := uuid.New()
	err := dc.MarkStatesRead(transactionID1, []string{state1.String()})
	require.NoError(t, err)

	state2 := tktypes.HexBytes("state2")
	transactionID2 := uuid.New()
	err = dc.MarkStatesSpending(transactionID2, []string{state2.String()})
	require.NoError(t, err)

	err = dc.ResetTransaction(transactionID1)
	require.NoError(t, err)

	assert.Len(t, dc.unFlushed.stateLocks, 1)
	assert.Equal(t, dc.unFlushed.stateLocks[0].State, state2)

}

func TestCheckEvalGTTimestamp(t *testing.T) {
	ctx, ss, _, done := newDBMockStateManager(t)
	defer done()

	contractAddress := tktypes.RandAddress()
	dc := ss.getDomainContext("domain1", *contractAddress)

	filterJSON :=
		`{"gt":[{"field":".created","value":1726545933211347000}],"limit":10,"sort":[".created"]}`
	var jq query.QueryJSON
	err := json.Unmarshal([]byte(filterJSON), &jq)
	assert.NoError(t, err)

	schema, err := newABISchema(ctx, "domain1", testABIParam(t, fakeCoinABI))
	require.NoError(t, err)
	labelSet := dc.ss.labelSetFor(schema)

	s := &components.State{
		ID:      tktypes.MustParseHexBytes("2eaf4727b7c7e9b3728b1344ac38ea6d8698603dc3b41d9458d7c011c20ce672"),
		Created: tktypes.TimestampFromUnix(1726545933211347000),
	}
	ls := filters.PassthroughValueSet{}
	addStateBaseLabels(ls, s.ID, s.Created)
	labelSet.labels[".created"] = nil

	match, err := filters.EvalQuery(dc.ctx, &jq, labelSet, ls)
	assert.NoError(t, err)
	assert.False(t, match)

}