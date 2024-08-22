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
package plugintk

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
)

func setupTransportTests(t *testing.T) (context.Context, *pluginExerciser[prototk.TransportMessage], *TransportAPIFunctions, TransportCallbacks, map[string]func(*prototk.TransportMessage), func()) {
	ctx, tc, tcDone := newTestController(t)

	/***** THIS PART AN IMPLEMENTATION WOULD DO ******/
	funcs := &TransportAPIFunctions{
		// Functions go here
	}
	waitForCallbacks := make(chan TransportCallbacks, 1)
	transport := NewTransport(func(callbacks TransportCallbacks) TransportAPI {
		// Implementation would construct an instance here to start handling the API calls from Paladin,
		// (rather than passing the callbacks to the test as we do here)
		waitForCallbacks <- callbacks
		return &TransportAPIBase{funcs}
	})
	/************************************************/

	// The rest is mocking the other side of the interface
	inOutMap := map[string]func(*prototk.TransportMessage){}
	pluginID := uuid.NewString()
	exerciser := newPluginExerciser(t, pluginID, &TransportMessageWrapper{}, inOutMap)
	tc.fakeTransportController = exerciser.controller

	domainDone := make(chan struct{})
	go func() {
		defer close(domainDone)
		transport.Run(pluginID, "unix:"+tc.socketFile)
	}()
	callbacks := <-waitForCallbacks

	return ctx, exerciser, funcs, callbacks, inOutMap, func() {
		checkPanic()
		transport.Stop()
		tcDone()
		<-domainDone
	}
}

func TestTransportCallback_ReceiveMessage(t *testing.T) {
	ctx, _, _, callbacks, inOutMap, done := setupTransportTests(t)
	defer done()

	inOutMap[fmt.Sprintf("%T", &prototk.TransportMessage_ReceiveMessage{})] = func(dm *prototk.TransportMessage) {
		dm.ResponseToTransport = &prototk.TransportMessage_ReceiveMessageRes{
			ReceiveMessageRes: &prototk.ReceiveMessageResponse{},
		}
	}
	_, err := callbacks.ReceiveMessage(ctx, &prototk.ReceiveMessageRequest{})
	assert.NoError(t, err)
}

func TestTransportFunction_ConfigureTransport(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupTransportTests(t)
	defer done()

	// ConfigureTransport - paladin to transport
	funcs.ConfigureTransport = func(ctx context.Context, cdr *prototk.ConfigureTransportRequest) (*prototk.ConfigureTransportResponse, error) {
		return &prototk.ConfigureTransportResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.TransportMessage) {
		req.RequestToTransport = &prototk.TransportMessage_ConfigureTransport{
			ConfigureTransport: &prototk.ConfigureTransportRequest{},
		}
	}, func(res *prototk.TransportMessage) {
		assert.IsType(t, &prototk.TransportMessage_ConfigureTransportRes{}, res.ResponseFromTransport)
	})
}

func TestTransportFunction_InitTransport(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupTransportTests(t)
	defer done()

	// InitTransport - paladin to transport
	funcs.InitTransport = func(ctx context.Context, cdr *prototk.InitTransportRequest) (*prototk.InitTransportResponse, error) {
		return &prototk.InitTransportResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.TransportMessage) {
		req.RequestToTransport = &prototk.TransportMessage_InitTransport{
			InitTransport: &prototk.InitTransportRequest{},
		}
	}, func(res *prototk.TransportMessage) {
		assert.IsType(t, &prototk.TransportMessage_InitTransportRes{}, res.ResponseFromTransport)
	})
}

func TestTransportFunction_SendMessage(t *testing.T) {
	_, exerciser, funcs, _, _, done := setupTransportTests(t)
	defer done()

	// InitTransport - paladin to transport
	funcs.SendMessage = func(ctx context.Context, cdr *prototk.SendMessageRequest) (*prototk.SendMessageResponse, error) {
		return &prototk.SendMessageResponse{}, nil
	}
	exerciser.doExchangeToPlugin(func(req *prototk.TransportMessage) {
		req.RequestToTransport = &prototk.TransportMessage_SendMessage{
			SendMessage: &prototk.SendMessageRequest{},
		}
	}, func(res *prototk.TransportMessage) {
		assert.IsType(t, &prototk.TransportMessage_SendMessageRes{}, res.ResponseFromTransport)
	})
}
