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
import io.kaleido.paladin.toolkit.ResourceLoader;

import java.io.IOException;
import java.util.Map;

public class CSDBondOrdersHelper {
    final PenteHelper pente;
    final JsonABI abi;
    final JsonHex.Address address;

    public record BondDetails(
            @JsonProperty
            String agent,
            @JsonProperty
            String dealer
    ) {
    }

    public static CSDBondOrdersHelper deploy(PenteHelper pente, String sender, Object inputs) throws IOException, InterruptedException {
        String bytecode = ResourceLoader.jsonResourceEntryText(
                CSDBondOrdersHelper.class.getClassLoader(),
                "contracts/private/CSDBondOrders.sol/CSDBondOrders.json",
                "bytecode"
        );
        JsonABI abi = JsonABI.fromJSONResourceEntry(
                CSDBondOrdersHelper.class.getClassLoader(),
                "contracts/private/CSDBondOrders.sol/CSDBondOrders.json",
                "abi"
        );
        var constructor = abi.getABIEntry("constructor", null);
        var address = pente.deploy(sender, bytecode, constructor.inputs(), inputs);
        return new CSDBondOrdersHelper(pente, abi, address);
    }

    private CSDBondOrdersHelper(PenteHelper pente, JsonABI abi, JsonHex.Address address) {
        this.pente = pente;
        this.abi = abi;
        this.address = address;
    }

    public JsonHex.Address address() {
        return address;
    }

    public JsonABI abi() {
        return abi;
    }

    public Testbed.TransactionResult deliverOrder(String sender, String isin, BondDetails details) throws IOException {
        var method = abi.getABIEntry("function", "deliverOrder");
        return pente.invoke(method.name(), method.inputs(), sender, address, Map.of(
                "isin", isin,
                "details", details
        ));
    }

    public Testbed.TransactionResult receiveOrder(String sender, String isin, BondDetails details) throws IOException {
        var method = abi.getABIEntry("function", "receiveOrder");
        return pente.invoke(method.name(), method.inputs(), sender, address, Map.of(
                "isin", isin,
                "details", details
        ));
    }

    public Testbed.TransactionResult prepareDistribution(String sender, String isin) throws IOException {
        var method = abi.getABIEntry("function", "prepareDistribution");
        return pente.invoke(method.name(), method.inputs(), sender, address, Map.of(
                "isin", isin
        ));
    }

    public Testbed.TransactionResult approveDistribution(String sender, String isin) throws IOException {
        var method = abi.getABIEntry("function", "approveDistribution");
        return pente.invoke(method.name(), method.inputs(), sender, address, Map.of(
                "isin", isin
        ));
    }
}
