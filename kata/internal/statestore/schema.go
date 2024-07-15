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

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"gorm.io/gorm/clause"
)

type SchemaType string

const (
	// ABI schema uses the same semantics as events for defining indexed fields (must be top-level)
	SchemaTypeABI SchemaType = "abi"
)

type SchemaEntity struct {
	Hash      HashID `gorm:"primaryKey;embedded;embeddedPrefix:hash_;"`
	Type      SchemaType
	Signature string
	Content   string
	Labels    []string `gorm:"type:text[]; serializer:json"`
}

type Schema interface {
	Type() SchemaType
	Persisted() *SchemaEntity
}

func (ss *stateStore) PersistSchema(ctx context.Context, s Schema) error {
	// TODO: Move to flush-writer
	err := ss.p.DB().
		Table("schemas").
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(s.Persisted()).
		Error
	if err != nil {
		return err
	}
	ss.abiSchemaCache.Set(s.Persisted().Hash.String(), s)
	return nil
}

func (ss *stateStore) GetSchema(ctx context.Context, hash *HashID) (Schema, error) {
	hk := hash.String()
	s, ok := ss.abiSchemaCache.Get(hk)
	if ok {
		return s, nil
	}

	var persisted *SchemaEntity
	err := ss.p.DB().
		Table("schemas").
		Where("hash_l = ?", hash.L.String()).
		Where("hash_h = ?", hash.H.String()).
		Limit(1).
		Find(&persisted).
		Error
	if err != nil || persisted == nil {
		return s, err
	}

	switch persisted.Type {
	case SchemaTypeABI:
		s, err = newABISchemaFromDB(ctx, persisted)
	default:
		err = i18n.NewError(ctx, msgs.MsgStateInvalidSchemaType, s.Type())
	}
	if err != nil {
		return nil, err
	}
	ss.abiSchemaCache.Set(s.Persisted().Hash.String(), s)
	return s, nil
}