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
	"fmt"
	"testing"
	"time"

	"github.com/hyperledger/firefly-common/pkg/ffresty"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/rpcbackend"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"github.com/stretchr/testify/assert"
)

var (
	toDomain      = "to-domain"
	testbedAddr   = "http://localhost:49600"
	grpcAddr      = "dns:localhost:49601"
	notaryName    = "notary"
	recipientName = "recipient"
)

func newTestDomain(t *testing.T) (context.Context, context.CancelFunc, *Noto, rpcbackend.Backend) {
	ctx := context.Background()
	domain, err := New(ctx, grpcAddr)
	assert.NoError(t, err)

	log.L(ctx).Infof("Listening for gRPC messages on %s", toDomain)
	err = domain.Listen(ctx, toDomain)
	assert.NoError(t, err)

	conf := ffresty.Config{URL: testbedAddr}
	rest := ffresty.NewWithConfig(ctx, conf)
	rpc := rpcbackend.NewRPCClient(rest)

	callCtx, cancelCtx := context.WithTimeout(ctx, 10*time.Second)
	cancel := func() {
		domain.Close()
		cancelCtx()
	}
	return callCtx, cancel, domain, rpc
}

func TestNoto(t *testing.T) {
	log.L(context.Background()).Infof("TestNoto")
	ctx, cancel, noto, rpc := newTestDomain(t)
	defer cancel()

	domainName := "noto_" + types.RandHex(8)
	log.L(ctx).Infof("Domain name = %s", domainName)
	factory := loadBuild(notoFactoryJSON)

	log.L(ctx).Infof("Deploying Noto factory")
	var factoryAddress string
	rpcerr := rpc.CallRPC(ctx, &factoryAddress, "testbed_deployBytecode",
		notaryName, factory.ABI, factory.Bytecode.String(), `{}`)
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
	}
	log.L(ctx).Infof("Noto factory deployed to %s", factoryAddress)

	log.L(ctx).Infof("Configuring Noto domain")
	var boolResult bool
	domainConfig := Config{
		FactoryAddress: factoryAddress,
	}
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_configureInit",
		domainName, domainConfig)
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
	}
	assert.True(t, boolResult)

	log.L(ctx).Infof("Deploying an instance of Noto")
	var notoAddress ethtypes.Address0xHex
	rpcerr = rpc.CallRPC(ctx, &notoAddress, "testbed_deploy",
		domainName, &NotoConstructorParams{Notary: notaryName})
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
	}
	log.L(ctx).Infof("Noto instance deployed to %s", notoAddress)

	log.L(ctx).Infof("Mint 100 from notary to notary")
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_invoke", &types.PrivateContractInvoke{
		From:     notaryName,
		To:       types.EthAddress(notoAddress),
		Function: *noto.Interface["mint"].ABI,
		Inputs: types.RawJSON(fmt.Sprintf(`{
			"to": "%s",
			"amount": 100
		}`, notaryName)),
	})
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
	}
	assert.True(t, boolResult)

	coins, err := noto.FindCoins(ctx, "{}")
	assert.NoError(t, err)
	assert.Len(t, coins, 1)
	assert.Equal(t, int64(100), coins[0].Amount.Int64())
	assert.Equal(t, notaryName, coins[0].Owner)

	log.L(ctx).Infof("Attempt mint from non-notary (should fail)")
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_invoke", &types.PrivateContractInvoke{
		From:     recipientName,
		To:       types.EthAddress(notoAddress),
		Function: *noto.Interface["mint"].ABI,
		Inputs: types.RawJSON(fmt.Sprintf(`{
			"to": "%s",
			"amount": 100
		}`, recipientName)),
	})
	assert.NotNil(t, rpcerr)
	assert.Error(t, rpcerr.Error(), "mint can only be initiated by notary")
	assert.True(t, boolResult)

	log.L(ctx).Infof("Transfer 150 from notary (should fail)")
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_invoke", &types.PrivateContractInvoke{
		From:     notaryName,
		To:       types.EthAddress(notoAddress),
		Function: *noto.Interface["transfer"].ABI,
		Inputs: types.RawJSON(fmt.Sprintf(`{
			"from": "%s",
			"to": "%s",
			"amount": 150
		}`, notaryName, recipientName)),
	})
	assert.NotNil(t, rpcerr)
	assert.Regexp(t, "insufficient funds", rpcerr.Error())

	log.L(ctx).Infof("Transfer 50 from notary to recipient")
	rpcerr = rpc.CallRPC(ctx, &boolResult, "testbed_invoke", &types.PrivateContractInvoke{
		From:     notaryName,
		To:       types.EthAddress(notoAddress),
		Function: *noto.Interface["transfer"].ABI,
		Inputs: types.RawJSON(fmt.Sprintf(`{
			"from": "%s",
			"to": "%s",
			"amount": 50
		}`, notaryName, recipientName)),
	})
	if rpcerr != nil {
		assert.NoError(t, rpcerr.Error())
	}

	coins, err = noto.FindCoins(ctx, "{}")
	assert.NoError(t, err)
	assert.Len(t, coins, 3)

	// This should have been spent
	// TODO: why does it still exist?
	assert.Equal(t, int64(100), coins[0].Amount.Int64())
	assert.Equal(t, notaryName, coins[0].Owner)

	// These are the expected coins after the transfer
	assert.Equal(t, int64(50), coins[1].Amount.Int64())
	assert.Equal(t, recipientName, coins[1].Owner)
	assert.Equal(t, int64(50), coins[2].Amount.Int64())
	assert.Equal(t, notaryName, coins[2].Owner)
}
