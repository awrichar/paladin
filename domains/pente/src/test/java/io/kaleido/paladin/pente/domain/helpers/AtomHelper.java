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

import com.fasterxml.jackson.annotation.JsonProperty;
import io.kaleido.paladin.testbed.Testbed;
import io.kaleido.paladin.toolkit.JsonABI;
import io.kaleido.paladin.toolkit.JsonHex;

import java.io.IOException;
import java.util.HashMap;

import static org.junit.jupiter.api.Assertions.assertNotNull;

public class AtomHelper {
    private static final int DEFAULT_POLL_TIMEOUT = 5000;

    final Testbed testbed;
    final JsonABI abi;
    final JsonHex.Address address;

    public record Operation(
            @JsonProperty
            JsonHex.Address contractAddress,
            @JsonProperty
            JsonHex.Bytes callData
    ) {
    }

    public AtomHelper(Testbed testbed, JsonHex.Address address) throws IOException {
        this.testbed = testbed;
        this.abi = JsonABI.fromJSONResourceEntry(
                AtomHelper.class.getClassLoader(),
                "contracts/shared/Atom.sol/Atom.json",
                "abi"
        );
        this.address = address;
    }

    public JsonHex.Address address() {
        return address;
    }

    public TestbedHelper.TransactionReceipt execute(String sender) throws IOException, InterruptedException {
        var txID = TestbedHelper.sendTransaction(testbed,
                new Testbed.TransactionInput(
                        "public",
                        "",
                        sender,
                        address,
                        new HashMap<>(),
                        abi,
                        "execute"
                ));
        return TestbedHelper.pollForReceipt(testbed, txID, DEFAULT_POLL_TIMEOUT);
    }
}
