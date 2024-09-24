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

package statestore

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/filters"
	"github.com/kaleido-io/paladin/core/internal/msgs"

	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type State struct {
	ID              tktypes.Bytes32    `json:"id"                  gorm:"primaryKey"`
	Created         tktypes.Timestamp  `json:"created"             gorm:"autoCreateTime:nano"`
	DomainName      string             `json:"domain"`
	Schema          tktypes.Bytes32    `json:"schema"`
	ContractAddress tktypes.EthAddress `json:"contractAddress"`
	Data            tktypes.RawJSON    `json:"data"`
	DataHash        tktypes.HexBytes   `json:"dataHash"`
	Labels          []*StateLabel      `json:"-"                   gorm:"foreignKey:state;references:id;"`
	Int64Labels     []*StateInt64Label `json:"-"                   gorm:"foreignKey:state;references:id;"`
	Confirmed       *StateConfirm      `json:"confirmed,omitempty" gorm:"foreignKey:state;references:data_hash;"`
	Spent           *StateSpend        `json:"spent,omitempty"     gorm:"foreignKey:state;references:data_hash;"`
	Locked          *StateLock         `json:"locked,omitempty"    gorm:"foreignKey:state;references:data_hash;"`
}

type StateUpsert struct {
	SchemaID string
	Data     tktypes.RawJSON
	DataHash tktypes.HexBytes
	Creating bool
	Spending bool
}

// StateWithLabels is a newly prepared state that has not yet been persisted
type StateWithLabels struct {
	*State
	LabelValues filters.ValueSet
}

type StateLabel struct {
	State tktypes.Bytes32 `gorm:"primaryKey"`
	Label string
	Value string
}

type StateInt64Label struct {
	State tktypes.Bytes32 `gorm:"primaryKey"`
	Label string
	Value int64
}

type StateUpdate struct {
	TXCreated *string
	TXSpent   *string
}

func (s *StateWithLabels) ValueSet() filters.ValueSet {
	return s.LabelValues
}

func (ss *stateStore) PersistState(ctx context.Context, domainName string, contractAddress tktypes.EthAddress, schemaID string, data tktypes.RawJSON, dataHash tktypes.HexBytes) (*StateWithLabels, error) {

	schema, err := ss.GetSchema(ctx, domainName, schemaID, true)
	if err != nil {
		return nil, err
	}

	s, err := schema.ProcessState(ctx, contractAddress, data, dataHash)
	if err != nil {
		return nil, err
	}

	op := ss.writer.newWriteOp(s.State.DomainName, contractAddress)
	op.states = []*StateWithLabels{s}
	ss.writer.queue(ctx, op)
	return s, op.flush(ctx)
}

func (ss *stateStore) GetState(ctx context.Context, domainName string, contractAddress tktypes.EthAddress, stateID string, failNotFound, withLabels bool) (*State, error) {
	id, err := tktypes.ParseBytes32Ctx(ctx, stateID)
	if err != nil {
		return nil, err
	}

	q := ss.p.DB().Table("states")
	if withLabels {
		q = q.Preload("Labels").Preload("Int64Labels")
	}
	var states []*State
	err = q.
		Where("domain_name = ?", domainName).
		Where("contract_address = ?", contractAddress).
		Where("id = ?", id).
		Limit(1).
		Find(&states).
		Error
	if err == nil && len(states) == 0 && failNotFound {
		return nil, i18n.NewError(ctx, msgs.MsgStateNotFound, id)
	}
	return states[0], err
}

// Built in fields all start with "." as that prevents them
// clashing with variable names in ABI structs ($ and _ are valid leading chars there)
var baseStateFields = map[string]filters.FieldResolver{
	".id":      filters.Bytes32Field("id"),
	".created": filters.TimestampField("created"),
}

func addStateBaseLabels(labelValues filters.PassthroughValueSet, id tktypes.Bytes32, createdAt tktypes.Timestamp) filters.PassthroughValueSet {
	labelValues[".id"] = id.HexString()
	labelValues[".created"] = int64(createdAt)
	return labelValues
}

type trackingLabelSet struct {
	labels map[string]*schemaLabelInfo
	used   map[string]*schemaLabelInfo
}

func (ft trackingLabelSet) ResolverFor(fieldName string) filters.FieldResolver {
	baseField := baseStateFields[fieldName]
	if baseField != nil {
		return baseField
	}
	f := ft.labels[fieldName]
	if f != nil {
		ft.used[fieldName] = f
		return f.resolver
	}
	return nil
}

func (ss *stateStore) labelSetFor(schema Schema) *trackingLabelSet {
	tls := trackingLabelSet{labels: make(map[string]*schemaLabelInfo), used: make(map[string]*schemaLabelInfo)}
	for _, fi := range schema.(labelInfoAccess).labelInfo() {
		tls.labels[fi.label] = fi
	}
	return &tls
}

func (ss *stateStore) FindStates(ctx context.Context, domainName string, contractAddress tktypes.EthAddress, schemaID string, query *query.QueryJSON, status StateStatusQualifier) (s []*State, err error) {
	_, s, err = ss.findStates(ctx, domainName, contractAddress, schemaID, query, status)
	return s, err
}

func (ss *stateStore) findStates(ctx context.Context, domainName string, contractAddress tktypes.EthAddress, schemaID string, jq *query.QueryJSON, status StateStatusQualifier, excluded ...*idOnly) (schema Schema, s []*State, err error) {
	if len(jq.Sort) == 0 {
		jq.Sort = []string{".created"}
	}

	schema, err = ss.GetSchema(ctx, domainName, schemaID, true)
	if err != nil {
		return nil, nil, err
	}

	tracker := ss.labelSetFor(schema)

	// Build the query
	db := ss.p.DB()
	q := filters.BuildGORM(ctx, jq, db.Table("states"), tracker)
	if q.Error != nil {
		return nil, nil, q.Error
	}

	// Add joins only for the fields actually used in the query
	for _, fi := range tracker.used {
		typeMod := ""
		if fi.labelType == labelTypeInt64 || fi.labelType == labelTypeBool {
			typeMod = "int64_"
		}
		q = q.Joins(fmt.Sprintf("INNER JOIN state_%[1]slabels AS %[2]s ON %[2]s.state = id AND %[2]s.label = ?", typeMod, fi.virtualColumn), fi.label)
	}

	q = q.Joins("Confirmed", db.Select("transaction")).
		Joins("Spent", db.Select("transaction")).
		Joins("Locked", db.Select("transaction")).
		Where("domain_name = ?", domainName).
		Where("contract_address = ?", contractAddress).
		Where("schema = ?", schema.Persisted().ID)

	if len(excluded) > 0 {
		q = q.Not(&State{}, excluded)
	}

	// Scope the query based of the qualifier
	q = q.Where(status.whereClause(db))

	var states []*State
	q = q.Find(&states)
	if q.Error != nil {
		return nil, nil, q.Error
	}
	return schema, states, nil
}

func (ss *stateStore) MarkConfirmed(ctx context.Context, domainName string, contractAddress tktypes.EthAddress, stateID string, transactionID uuid.UUID) error {
	id, err := tktypes.ParseHexBytes(ctx, stateID)
	if err != nil {
		return err
	}

	op := ss.writer.newWriteOp(domainName, contractAddress)
	op.stateConfirms = []*StateConfirm{
		{State: id, Transaction: transactionID},
	}

	ss.writer.queue(ctx, op)
	return op.flush(ctx)
}

func (ss *stateStore) MarkSpent(ctx context.Context, domainName string, contractAddress tktypes.EthAddress, stateID string, transactionID uuid.UUID) error {
	id, err := tktypes.ParseHexBytes(ctx, stateID)
	if err != nil {
		return err
	}

	op := ss.writer.newWriteOp(domainName, contractAddress)
	op.stateSpends = []*StateSpend{
		{State: id, Transaction: transactionID},
	}

	ss.writer.queue(ctx, op)
	return op.flush(ctx)
}

func (ss *stateStore) MarkLocked(ctx context.Context, domainName string, contractAddress tktypes.EthAddress, stateID string, transactionID uuid.UUID, creating, spending bool) error {
	id, err := tktypes.ParseHexBytes(ctx, stateID)
	if err != nil {
		return err
	}

	op := ss.writer.newWriteOp(domainName, contractAddress)
	op.stateLocks = []*StateLock{
		{State: id, Transaction: transactionID, Creating: creating, Spending: spending},
	}

	ss.writer.queue(ctx, op)
	return op.flush(ctx)
}

func (ss *stateStore) ResetTransaction(ctx context.Context, domainName string, contractAddress tktypes.EthAddress, transactionID uuid.UUID) error {
	op := ss.writer.newWriteOp(domainName, contractAddress)
	op.transactionLockDeletes = []uuid.UUID{transactionID}

	ss.writer.queue(ctx, op)
	return op.flush(ctx)
}
