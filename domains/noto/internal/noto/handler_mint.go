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

package noto

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/domains/noto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/domain"
	pb "github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type mintHandler struct {
	noto *Noto
}

func (h *mintHandler) ValidateParams(ctx context.Context, params string) (interface{}, error) {
	var mintParams types.MintParams
	if err := json.Unmarshal([]byte(params), &mintParams); err != nil {
		return nil, err
	}
	if mintParams.To == "" {
		return nil, fmt.Errorf("parameter 'to' is required")
	}
	if mintParams.Amount.BigInt().Sign() != 1 {
		return nil, fmt.Errorf("parameter 'amount' must be greater than 0")
	}
	return &mintParams, nil
}

func (h *mintHandler) Init(ctx context.Context, tx *types.ParsedTransaction, req *pb.InitTransactionRequest) (*pb.InitTransactionResponse, error) {
	params := tx.Params.(*types.MintParams)

	if req.Transaction.From != tx.DomainConfig.NotaryLookup {
		return nil, fmt.Errorf("mint can only be initiated by notary")
	}
	return &pb.InitTransactionResponse{
		RequiredVerifiers: []*pb.ResolveVerifierRequest{
			{
				Lookup:    tx.DomainConfig.NotaryLookup,
				Algorithm: algorithms.ECDSA_SECP256K1_PLAINBYTES,
			},
			{
				Lookup:    params.To,
				Algorithm: algorithms.ECDSA_SECP256K1_PLAINBYTES,
			},
		},
	}, nil
}

func (h *mintHandler) Assemble(ctx context.Context, tx *types.ParsedTransaction, req *pb.AssembleTransactionRequest) (*pb.AssembleTransactionResponse, error) {
	params := tx.Params.(*types.MintParams)

	notary := domain.FindVerifier(tx.DomainConfig.NotaryLookup, algorithms.ECDSA_SECP256K1_PLAINBYTES, req.ResolvedVerifiers)
	if notary == nil || notary.Verifier != tx.DomainConfig.NotaryAddress {
		return nil, fmt.Errorf("notary resolved to unexpected address")
	}
	to := domain.FindVerifier(params.To, algorithms.ECDSA_SECP256K1_PLAINBYTES, req.ResolvedVerifiers)
	if to == nil {
		return nil, fmt.Errorf("error verifying recipient address")
	}
	toAddress, err := ethtypes.NewAddress(to.Verifier)
	if err != nil {
		return nil, err
	}

	_, outputStates, err := h.noto.prepareOutputs(*toAddress, params.Amount)
	if err != nil {
		return nil, err
	}

	return &pb.AssembleTransactionResponse{
		AssemblyResult: pb.AssembleTransactionResponse_OK,
		AssembledTransaction: &pb.AssembledTransaction{
			OutputStates: outputStates,
		},
		AttestationPlan: []*pb.AttestationRequest{
			// Notary will endorse the assembled transaction (by submitting to the ledger)
			// Note no  additional attestation using req.Transaction.From, because it is guaranteed to be the notary
			{
				Name:            "notary",
				AttestationType: pb.AttestationType_ENDORSE,
				Algorithm:       algorithms.ECDSA_SECP256K1_PLAINBYTES,
				Parties:         []string{tx.DomainConfig.NotaryLookup},
			},
		},
	}, nil
}

func (h *mintHandler) validateAmounts(params *types.MintParams, coins *gatheredCoins) error {
	if len(coins.inCoins) > 0 {
		return fmt.Errorf("invalid inputs to 'mint': %v", coins.inCoins)
	}
	if coins.outTotal.Cmp(params.Amount.BigInt()) != 0 {
		return fmt.Errorf("invalid amount for 'mint'")
	}
	return nil
}

func (h *mintHandler) Endorse(ctx context.Context, tx *types.ParsedTransaction, req *pb.EndorseTransactionRequest) (*pb.EndorseTransactionResponse, error) {
	params := tx.Params.(*types.MintParams)
	coins, err := h.noto.gatherCoins(ctx, req.Inputs, req.Outputs)
	if err != nil {
		return nil, err
	}
	if err := h.validateAmounts(params, coins); err != nil {
		return nil, err
	}
	return &pb.EndorseTransactionResponse{
		EndorsementResult: pb.EndorseTransactionResponse_ENDORSER_SUBMIT,
	}, nil
}

func (h *mintHandler) Prepare(ctx context.Context, tx *types.ParsedTransaction, req *pb.PrepareTransactionRequest) (*pb.PrepareTransactionResponse, error) {
	outputs := make([]string, len(req.OutputStates))
	for i, state := range req.OutputStates {
		outputs[i] = state.Id
	}

	params := map[string]interface{}{
		"outputs":   outputs,
		"signature": "0x", // no signature, because requester AND submitter are always the notary
		"data":      req.Transaction.TransactionId,
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	functionJSON, err := json.Marshal(h.noto.contractABI.Functions()["mint"])
	if err != nil {
		return nil, err
	}

	return &pb.PrepareTransactionResponse{
		Transaction: &pb.BaseLedgerTransaction{
			FunctionAbiJson: string(functionJSON),
			ParamsJson:      string(paramsJSON),
		},
	}, nil
}
