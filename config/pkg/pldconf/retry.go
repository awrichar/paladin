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

package pldconf

import "github.com/kaleido-io/paladin/config/pkg/confutil"

type RetryConfig struct {
	InitialDelay *string  `json:"initialDelay"`
	MaxDelay     *string  `json:"maxDelay"`
	Factor       *float64 `json:"factor"`
}

type RetryConfigWithMax struct {
	RetryConfig
	MaxAttempts *int    `json:"maxAttempts"`
	MaxTime     *string `json:"maxTime"`
}

var RetryDefaults = &RetryConfigWithMax{
	RetryConfig: RetryConfig{
		InitialDelay: confutil.P("250ms"),
		MaxDelay:     confutil.P("30s"),
		Factor:       confutil.P(2.0),
	},
	MaxAttempts: confutil.P(3),
}