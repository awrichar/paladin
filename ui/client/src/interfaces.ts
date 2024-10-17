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

export interface IApplicationContext {
  lastBlockWithTransactions: number
  errorMessage?: string
}

export interface ITransaction {
  hash: string
  blockNumber: number
  transactionIndex: number
  from: string
  nonce: number
  contractAddress?: string
  result: string
}

export interface IEvent {
  blockNumber: number
  transactionIndex: number
  logIndex: number
  transactionHash: string
  signature: string
}

export interface IRegistryEntry {
  registry: string
  id: string
  name: string
  active: boolean
  properties: {
    [key: string]: string
  }
}