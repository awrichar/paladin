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
package plugins

import (
	"context"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type DomainManagerToDomain interface {
	plugintk.DomainAPI
	Initialized()
}

// The interface the rest of Paladin uses to integrate with domain plugins
type DomainRegistration interface {
	ConfiguredDomains() map[string]*PluginConfig
	DomainRegistered(name string, id uuid.UUID, toDomain DomainManagerToDomain) (fromDomain plugintk.DomainCallbacks, err error)
}

// The gRPC stream connected to by domain plugins
func (pc *pluginController) ConnectDomain(stream prototk.PluginController_ConnectDomainServer) error {
	handler := newPluginHandler(pc, pc.domainPlugins, stream,
		&plugintk.DomainMessageWrapper{},
		func(plugin *plugin[prototk.DomainMessage], toPlugin managerToPlugin[prototk.DomainMessage]) (pluginToManager pluginToManager[prototk.DomainMessage], err error) {
			br := &domainBridge{
				plugin:     plugin,
				pluginType: plugin.def.Plugin.PluginType.String(),
				pluginName: plugin.name,
				pluginId:   plugin.id.String(),
				toPlugin:   toPlugin,
			}
			br.manager, err = pc.domainManager.DomainRegistered(plugin.name, plugin.id, br)
			if err != nil {
				return nil, err
			}
			return br, nil
		})
	return handler.serve()
}

type domainBridge struct {
	plugin     *plugin[prototk.DomainMessage]
	pluginType string
	pluginName string
	pluginId   string
	toPlugin   managerToPlugin[prototk.DomainMessage]
	manager    plugintk.DomainCallbacks
}

// DomainManager calls this when it is satisfied the domain is fully initialized.
// WaitForStart will block until this is done.
func (br *domainBridge) Initialized() {
	br.plugin.notifyInitialized()
}

// requests to callbacks in the domain manager
func (br *domainBridge) RequestReply(ctx context.Context, reqMsg plugintk.PluginMessage[prototk.DomainMessage]) (resFn func(plugintk.PluginMessage[prototk.DomainMessage]), err error) {
	switch req := reqMsg.Message().RequestFromDomain.(type) {
	case *prototk.DomainMessage_FindAvailableStates:
		return callManagerImpl(ctx, req.FindAvailableStates,
			br.manager.FindAvailableStates,
			func(resMsg *prototk.DomainMessage, res *prototk.FindAvailableStatesResponse) {
				resMsg.ResponseToDomain = &prototk.DomainMessage_FindAvailableStatesRes{
					FindAvailableStatesRes: res,
				}
			},
		)
	default:
		return nil, i18n.NewError(ctx, msgs.MsgPluginBadRequestBody, req)
	}
}

func (br *domainBridge) ConfigureDomain(ctx context.Context, req *prototk.ConfigureDomainRequest) (res *prototk.ConfigureDomainResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_ConfigureDomain{ConfigureDomain: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_ConfigureDomainRes); ok {
				res = r.ConfigureDomainRes
			}
			return res != nil
		},
	)
	return
}

func (br *domainBridge) InitDomain(ctx context.Context, req *prototk.InitDomainRequest) (res *prototk.InitDomainResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_InitDomain{InitDomain: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_InitDomainRes); ok {
				res = r.InitDomainRes
			}
			return res != nil
		},
	)
	return
}

func (br *domainBridge) InitDeploy(ctx context.Context, req *prototk.InitDeployRequest) (res *prototk.InitDeployResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_InitDeploy{InitDeploy: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_InitDeployRes); ok {
				res = r.InitDeployRes
			}
			return res != nil
		},
	)
	return
}

func (br *domainBridge) PrepareDeploy(ctx context.Context, req *prototk.PrepareDeployRequest) (res *prototk.PrepareDeployResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_PrepareDeploy{PrepareDeploy: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_PrepareDeployRes); ok {
				res = r.PrepareDeployRes
			}
			return res != nil
		},
	)
	return
}

func (br *domainBridge) InitTransaction(ctx context.Context, req *prototk.InitTransactionRequest) (res *prototk.InitTransactionResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_InitTransaction{InitTransaction: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_InitTransactionRes); ok {
				res = r.InitTransactionRes
			}
			return res != nil
		},
	)
	return
}

func (br *domainBridge) AssembleTransaction(ctx context.Context, req *prototk.AssembleTransactionRequest) (res *prototk.AssembleTransactionResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_AssembleTransaction{AssembleTransaction: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_AssembleTransactionRes); ok {
				res = r.AssembleTransactionRes
			}
			return res != nil
		},
	)
	return
}

func (br *domainBridge) EndorseTransaction(ctx context.Context, req *prototk.EndorseTransactionRequest) (res *prototk.EndorseTransactionResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_EndorseTransaction{EndorseTransaction: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_EndorseTransactionRes); ok {
				res = r.EndorseTransactionRes
			}
			return res != nil
		},
	)
	return
}

func (br *domainBridge) PrepareTransaction(ctx context.Context, req *prototk.PrepareTransactionRequest) (res *prototk.PrepareTransactionResponse, err error) {
	err = br.toPlugin.RequestReply(ctx,
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) {
			dm.Message().RequestToDomain = &prototk.DomainMessage_PrepareTransaction{PrepareTransaction: req}
		},
		func(dm plugintk.PluginMessage[prototk.DomainMessage]) bool {
			if r, ok := dm.Message().ResponseFromDomain.(*prototk.DomainMessage_PrepareTransactionRes); ok {
				res = r.PrepareTransactionRes
			}
			return res != nil
		},
	)
	return
}