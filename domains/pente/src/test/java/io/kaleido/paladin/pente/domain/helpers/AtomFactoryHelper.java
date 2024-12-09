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

package io.kaleido.paladin.pente.domain.helpers;

import com.fasterxml.jackson.databind.ObjectMapper;
import io.kaleido.paladin.testbed.Testbed;
import io.kaleido.paladin.toolkit.JsonABI;
import io.kaleido.paladin.toolkit.JsonHex;
import io.kaleido.paladin.toolkit.ResourceLoader;

import java.io.IOException;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertFalse;

public class AtomFactoryHelper {
    private static final int DEFAULT_POLL_TIMEOUT = 5000;

    final Testbed testbed;
    final JsonABI abi;
    final JsonHex.Address address;

    public static AtomFactoryHelper deploy(Testbed testbed, String sender, Object inputs) throws IOException {
        String bytecode = ResourceLoader.jsonResourceEntryText(
                AtomFactoryHelper.class.getClassLoader(),
                "contracts/shared/Atom.sol/AtomFactory.json",
                "bytecode"
        );
        JsonABI abi = JsonABI.fromJSONResourceEntry(
                AtomFactoryHelper.class.getClassLoader(),
                "contracts/shared/Atom.sol/AtomFactory.json",
                "abi"
        );
        String address = testbed.getRpcClient().request("testbed_deployBytecode",
                sender,
                abi,
                bytecode,
                inputs);
        return new AtomFactoryHelper(testbed, abi, JsonHex.addressFrom(address));
    }

    private AtomFactoryHelper(Testbed testbed, JsonABI abi, JsonHex.Address address) {
        this.testbed = testbed;
        this.abi = abi;
        this.address = address;
    }

    public JsonHex.Address address() {
        return address;
    }

    public AtomHelper create(String sender, List<AtomHelper.Operation> operations) throws IOException, InterruptedException {
        var txID = TestbedHelper.sendTransaction(testbed, new Testbed.TransactionInput(
                "public",
                "",
                sender,
                address,
                Map.of("operations", operations),
                abi,
                "create"
        ));
        var receipt = TestbedHelper.pollForReceipt(testbed, txID, DEFAULT_POLL_TIMEOUT);
        if (receipt != null) {
            List<HashMap<String, Object>> events = testbed.getRpcClient().request("bidx_decodeTransactionEvents",
                    receipt.transactionHash(),
                    abi,
                    "");
            var deployEvent = events.stream().filter(ev -> ev.get("soliditySignature").toString().startsWith("event AtomDeployed")).findFirst();
            assertFalse(deployEvent.isEmpty());
            var deployEventData = new ObjectMapper().convertValue(deployEvent.get().get("data"), HashMap.class);
            var atomAddress = JsonHex.addressFrom(deployEventData.get("addr").toString());
            return new AtomHelper(testbed, atomAddress);
        }
        return null;
    }
}
