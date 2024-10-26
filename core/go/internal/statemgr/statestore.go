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
	"sync"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/core/pkg/persistence"
	"github.com/kaleido-io/paladin/toolkit/pkg/cache"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcserver"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type stateManager struct {
	p                 persistence.Persistence
	bgCtx             context.Context
	cancelCtx         context.CancelFunc
	conf              *pldconf.StateStoreConfig
	domainManager     components.DomainManager
	abiSchemaCache    cache.Cache[string, components.Schema]
	rpcModule         *rpcserver.RPCModule
	domainContextLock sync.Mutex
	domainContexts    map[uuid.UUID]*domainContext
}

var SchemaCacheDefaults = &pldconf.CacheConfig{
	Capacity: confutil.P(1000),
}

func NewStateManager(ctx context.Context, conf *pldconf.StateStoreConfig, p persistence.Persistence) components.StateManager {
	ss := &stateManager{
		p:              p,
		conf:           conf,
		abiSchemaCache: cache.NewCache[string, components.Schema](&conf.SchemaCache, SchemaCacheDefaults),
		domainContexts: make(map[uuid.UUID]*domainContext),
	}
	ss.bgCtx, ss.cancelCtx = context.WithCancel(ctx)
	return ss
}

func (ss *stateManager) PreInit(c components.PreInitComponents) (*components.ManagerInitResult, error) {
	ss.initRPC()
	return &components.ManagerInitResult{
		RPCModules: []*rpcserver.RPCModule{ss.rpcModule},
	}, nil
}

func (ss *stateManager) PostInit(c components.AllComponents) error {
	ss.domainManager = c.DomainManager()
	return nil
}

func (ss *stateManager) Start() error {
	return nil
}

func (ss *stateManager) Stop() {
	ss.cancelCtx()
}

// Confirmation and spending records are not managed via the in-memory cached model of states,
// rather they are written to the database in the DB transaction of the block indexer,
// such that any failure in that DB transaction will be atomic with the writing of the records.
//
// By their nature they happen asynchronously from the coordination and assembly of new
// transactions, and it is the private transaction manager's responsibility to process
// them when notified post-commit about the domains/transactions that are affected and
// might have in-memory processing.
//
// As such, no attempt is made to coordinate these changes with the queries that might
// be happening concurrently against the database, and after commit of these changes
// might find new states become available and/or states marked locked for spending
// become fully unavailable.
func (ss *stateManager) WriteStateFinalizations(ctx context.Context, dbTX *gorm.DB, spends []*pldapi.StateSpend, reads []*pldapi.StateRead, confirms []*pldapi.StateConfirm) (err error) {
	if len(spends) > 0 {
		err = dbTX.
			WithContext(ctx).
			Table("state_spends").
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(spends).
			Error
	}
	if err == nil && len(reads) > 0 {
		err = dbTX.
			WithContext(ctx).
			Table("state_reads").
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(reads).
			Error
	}
	if err == nil && len(confirms) > 0 {
		err = dbTX.
			WithContext(ctx).
			Table("state_confirms").
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(confirms).
			Error
	}
	return err
}

func (ss *stateManager) GetTransactionStates(ctx context.Context, dbTX *gorm.DB, txID uuid.UUID) (*pldapi.TransactionStates, error) {

	// We query from the states table, joining in the other fields
	var states []*transactionStateRecord
	err := dbTX.
		WithContext(ctx).
		// This query joins across three tables in a single query - pushing the complexity to the DB.
		// The reason we have three tables is to make the queries for available states simpler.
		Raw(`SELECT * from "states" RIGHT JOIN ( `+
			`SELECT "transaction", "state", 'spent'     AS "record_type" FROM "state_spends"   WHERE "transaction" = ? UNION ALL `+
			`SELECT "transaction", "state", 'read'      AS "record_type" FROM "state_reads"    WHERE "transaction" = ? UNION ALL `+
			`SELECT "transaction", "state", 'confirmed' AS "record_type" FROM "state_confirms" WHERE "transaction" = ? ) "records" `+
			`ON "states"."id" = "records"."state"`,
			txID, txID, txID).
		Scan(&states).
		Error
	if err != nil {
		return nil, err
	}
	txStates := &pldapi.TransactionStates{}
	for _, s := range states {
		switch s.RecordType {
		case "spent":
			if s.ID == nil {
				return nil, i18n.NewError(ctx, msgs.MsgStateTxMissingDataForState, s.SpentState, txID, "spent")
			}
			txStates.Spent = append(txStates.Spent, &s.StateBase)
		case "read":
			if s.ID == nil {
				return nil, i18n.NewError(ctx, msgs.MsgStateTxMissingDataForState, s.ReadState, txID, "read")
			}
			txStates.Read = append(txStates.Read, &s.StateBase)
		case "confirmed":
			if s.ID == nil {
				return nil, i18n.NewError(ctx, msgs.MsgStateTxMissingDataForState, s.ConfirmedState, txID, "confirmed")
			}
			txStates.Confirmed = append(txStates.Confirmed, &s.StateBase)
		}
	}
	return txStates, nil

}
