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

package filters

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"math/big"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/kata/pkg/types"
)

type Int64Field string

func (sf Int64Field) SQLColumn() string {
	return (string)(sf)
}

func (sf Int64Field) SupportsLIKE() bool {
	return false
}

func (sf Int64Field) SQLValue(ctx context.Context, jsonValue types.RawJSON) (driver.Value, error) {
	if jsonValue.IsNil() {
		return nil, nil
	}
	var untyped interface{}
	err := json.Unmarshal(jsonValue, &untyped)
	if err != nil {
		return nil, err
	}
	switch v := untyped.(type) {
	case string:
		bi, ok := new(big.Int).SetString(v, 0)
		if ok && bi.IsInt64() {
			return bi.Int64(), nil
		}
		return nil, i18n.NewError(ctx, msgs.MsgFiltersValueInvalidForInt64, v)
	case float64:
		return (int64)(v), nil
	case bool:
		if v {
			return (int64)(1), nil
		}
		return (int64)(0), nil
	default:
		return nil, i18n.NewError(ctx, msgs.MsgFiltersValueInvalidForInt64, string(jsonValue))
	}
}