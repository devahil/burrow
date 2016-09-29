// Copyright 2015, 2016 Eris Industries (UK) Ltd.
// This file is part of Eris-RT

// Eris-RT is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Eris-RT is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.

// You should have received a copy of the GNU General Public License
// along with Eris-RT.  If not, see <http://www.gnu.org/licenses/>.

package client

import (
	"encoding/json"
	"fmt"

	"github.com/tendermint/go-rpc/client"
	"github.com/tendermint/go-wire"

	log "github.com/eris-ltd/eris-logger"

	"github.com/eris-ltd/eris-bd/txs"
	ctypes "github.com/eris-ltd/eris-db/rpc/tendermint/core/types"
)

type Confirmation {
	BlockHash []byte
	Event     *txs.EventData
	Exception error
}

// NOTE [ben] Compiler check to ensure ErisNodeClient successfully implements
// eris-db/client.NodeClient
var _ NodeWebsocketClient = (*ErisNodeWebsocketClient)(nil)

type ErisNodeWebsocketClient struct {
	// TODO: assert no memory leak on closing with open websocket
	tendermintWebsocket *rpcclient.WSClient
}

// Subscribe to an eventid
func (erisNodeWebsocketClient *ErisNodeWebsocketClient) Subscribe(eventid string) error {
	// TODO we can in the background listen to the subscription id and remember it to ease unsubscribing later.
	return erisNodeWebsocketClient.tendermintWebsocket.Subscribe(eventid)
}

// Unsubscribe from an eventid
func (erisNodeWebsocketClient *ErisNodeWebsocketClient) Unsubscribe(subscriptionId string) error {
	return erisNodeWebsocketClient.tendermintWebsocket.Unsubscribe(subscriptionId)
}

// Returns a channel that will receive a confirmation with a result or the exception that
// has been confirmed; or an error is returned and the confirmation channel is nil.
func (erisNodeWebsocketClient *ErisNodeWebsocketClient) WaitForConfirmation(eventid string) (chan Confirmation, error) {
	// Setup the confirmation channel to be returned
	confirmationChannel := make(chan Confirmation, 1)
	var latestBlockHash []byte

	eid := txs.EventStringAccInput(inputAddr)

	// Read the incoming events
	go func() {
		var err error
		for {
			resultBytes := <- erisNodeWebsocketClient.tendermintWebsocket.ResultsCh
			result := new(ctypes.ErisDBResult)
			if wire.readJSONPtr(result, r, &err); err != nil {
				// keep calm and carry on
				log.Errorf("eris-client - Failed to unmarshal json bytes for websocket event: %s", err)
				continue
			}
			
			event, ok := (*result).(*ctypes.ResultEvent)
			if !ok {
				// keep calm and carry on
				log.Error("eris-client - Failed to cast to ResultEvent for websocket event")
				continue
			}
			
			blockData, ok := event.Data.(txs.EventDataNewBlock)
			if ok {
				latestBlockHash = blockData.Block.Hash()
				log.WithFields(log.Field{
					"new block": blockData.Block,
					"latest hash": latestBlockHash,
				}).Debug("Registered new block")
				continue
			}
			
			// we don't accept events unless they came after a new block (ie. in)
			if latestBlockHash == nil {
				continue
			}

			if event.Event != eid {
				log.Warnf("Received unsolicited event! Got %s, expected %s\n", result.Event, eid)
				continue
			}

			data, ok := result.Data.(txs.EventDataTx)
			if !ok {
				// We are on the lookout for EventDataTx
				confirmationChannel <- Confirmation{
					BlockHash: latestBlockHash,
					Event: nil // or result.Data ?
					Exception: fmt.Errorf("response error: expected result.Data to be *types.EventDataTx")
				}
			}
		}
	}

}

func (erisNodeWebsocketClient *ErisNodeWebsocketClient) Close() {
	if erisNodeWebsocketClient.tendermintWebsocket != nil {
		erisNodeWebsocketClient.tendermintWebsocket.Stop()
	}
}

func (erisNodeWebsocketClient *ErisNodeWebsocketClient) assertNoErrors() error {
	if erisNodeWebsocketClient.tendermintWebsocket != nil {
		select {
		case err := <-erisNodeWebsocketClient.tendermintWebsocket.ErrorCh:
			return err
		default:
			return nil
	} else {
		return fmt.Errorf("Eris-client")
	}
}