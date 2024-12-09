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

package io.kaleido.paladin.pente.domain;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.kaleido.paladin.pente.domain.PenteConfiguration.GroupTupleJSON;
import io.kaleido.paladin.pente.domain.helpers.*;
import io.kaleido.paladin.testbed.Testbed;
import io.kaleido.paladin.toolkit.*;
import org.junit.jupiter.api.Test;

import java.util.HashMap;
import java.util.List;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.*;

public class CSDBondTest {

    private final Testbed.Setup testbedSetup = new Testbed.Setup(
            "../../core/go/db/migrations/sqlite",
            "build/testbed.java-bond.log",
            5000);

    JsonHex.Address deployPenteFactory() throws Exception {
        try (Testbed deployBed = new Testbed(testbedSetup)) {
            String factoryBytecode = ResourceLoader.jsonResourceEntryText(
                    this.getClass().getClassLoader(),
                    "contracts/domains/pente/PenteFactory.sol/PenteFactory.json",
                    "bytecode"
            );
            JsonABI factoryABI = JsonABI.fromJSONResourceEntry(
                    this.getClass().getClassLoader(),
                    "contracts/domains/pente/PenteFactory.sol/PenteFactory.json",
                    "abi"
            );
            String contractAddr = deployBed.getRpcClient().request("testbed_deployBytecode",
                    "deployer",
                    factoryABI,
                    factoryBytecode,
                    new HashMap<String, String>());
            return new JsonHex.Address(contractAddr);
        }
    }

    JsonHex.Address deployNotoFactory() throws Exception {
        try (Testbed deployBed = new Testbed(testbedSetup)) {
            String factoryBytecode = ResourceLoader.jsonResourceEntryText(
                    this.getClass().getClassLoader(),
                    "contracts/domains/noto/NotoFactory.sol/NotoFactory.json",
                    "bytecode"
            );
            JsonABI factoryABI = JsonABI.fromJSONResourceEntry(
                    this.getClass().getClassLoader(),
                    "contracts/domains/noto/NotoFactory.sol/NotoFactory.json",
                    "abi"
            );
            String contractAddr = deployBed.getRpcClient().request("testbed_deployBytecode",
                    "deployer",
                    factoryABI,
                    factoryBytecode,
                    new HashMap<String, String>());
            return new JsonHex.Address(contractAddr);
        }
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    record StateSchema(
            @JsonProperty
            JsonHex.Bytes32 id,
            @JsonProperty
            String signature
    ) {
    }

    @Test
    void testCSDBond() throws Exception {
        JsonHex.Address penteFactoryAddress = deployPenteFactory();
        JsonHex.Address notoFactoryAddress = deployNotoFactory();
        try (Testbed testbed = new Testbed(testbedSetup,
                new Testbed.ConfigDomain(
                        "pente",
                        penteFactoryAddress,
                        new Testbed.ConfigPlugin("jar", "", PenteDomainFactory.class.getName()),
                        new HashMap<>()
                ),
                new Testbed.ConfigDomain(
                        "noto",
                        notoFactoryAddress,
                        new Testbed.ConfigPlugin("c-shared", "noto", ""),
                        new HashMap<>()
                )
        )) {

            String cashIssuer = "cashIssuer";
            String csdRequester = "csdRequester";
            String csdChecker = "csdChecker";
            String agent = "agent";
            String dealer = "dealer";

            String csdRequesterAddress = testbed.getRpcClient().request("testbed_resolveVerifier",
                    csdRequester, Algorithms.ECDSA_SECP256K1, Verifiers.ETH_ADDRESS);
            String agentAddress = testbed.getRpcClient().request("testbed_resolveVerifier",
                    agent, Algorithms.ECDSA_SECP256K1, Verifiers.ETH_ADDRESS);
            String dealerAddress = testbed.getRpcClient().request("testbed_resolveVerifier",
                    dealer, Algorithms.ECDSA_SECP256K1, Verifiers.ETH_ADDRESS);

            var mapper = new ObjectMapper();
            List<JsonNode> notoSchemas = testbed.getRpcClient().request("pstate_listSchemas",
                    "noto");
            assertEquals(2, notoSchemas.size());
            StateSchema notoSchema = null;
            for (var i = 0; i < 2; i++) {
                var schema = mapper.convertValue(notoSchemas.get(i), StateSchema.class);
                if (schema.signature().equals("type=NotoCoin(bytes32 salt,string owner,uint256 amount),labels=[owner,amount]")) {
                    notoSchema = schema;
                } else {
                    assertEquals("type=TransactionData(bytes32 salt,bytes data),labels=[]", schema.signature());
                }
            }
            assertNotNull(notoSchema);

            // Create the atom factory on the base ledger
            var atomFactory = AtomFactoryHelper.deploy(testbed, csdRequester, null);

            // Create Noto cash token
            var notoCash = NotoHelper.deploy("noto", cashIssuer, testbed,
                    new NotoHelper.ConstructorParams(
                            cashIssuer + "@node1",
                            null,
                            true));
            assertFalse(notoCash.address().isBlank());

            // Issue cash to investors
            notoCash.mint(cashIssuer, dealer, 100000);

            GroupTupleJSON csdGroup = new GroupTupleJSON(
                    JsonHex.randomBytes32(), new String[]{csdRequester});

            // Create the privacy groups
            var csdInstance = PenteHelper.newPrivacyGroup(
                    "pente", csdRequester, testbed, csdGroup, true);
            assertFalse(csdInstance.address().isBlank());

            // Deploy private investor list to the CSD privacy group
            var investorList = InvestorListHelper.deploy(csdInstance, csdRequester, new HashMap<>(Map.of(
                    "initialOwner", csdRequesterAddress
            )));

            // Deploy private bond tracker to the CSD privacy group
            var bondTracker = CSDBondTrackerHelper.deploy(csdInstance, csdRequester, new HashMap<>(Map.of(
                    "name", "BOND",
                    "symbol", "BOND",
                    "transferPolicy_", investorList.address()
            )));

            // Deploy private bond orders contract to the issuer privacy group
            var bondOrders = CSDBondOrdersHelper.deploy(csdInstance, csdRequester, new HashMap<>(Map.of(
                    "bondTracker_", bondTracker.address()
            )));
            TestbedHelper.storeABI(testbed, bondOrders.abi());

            // Perform bond pre-issuance workflow
            final String isin = "ZZ0123456AB0";
            bondTracker.approveRequest(csdChecker);
            bondTracker.setISIN(csdRequester, isin);
            bondTracker.approveISIN(csdChecker);

            // Create Noto bond token
            var notoBond = NotoHelper.deploy("noto", csdRequester, testbed,
                    new NotoHelper.ConstructorParams(
                            csdRequester + "@node1",
                            new NotoHelper.HookParams(
                                    csdInstance.address(),
                                    bondTracker.address(),
                                    csdGroup),
                            false));
            assertFalse(notoBond.address().isBlank());

            // Issue bond to agent
            bondTracker.prepareIssuance(csdRequester);
            notoBond.mint(csdChecker, agent, 1000);

            // Validate Noto balances
            var notoCashStates = notoCash.queryStates(notoSchema.id, null);
            assertEquals(1, notoCashStates.size());
            assertEquals("100000", notoCashStates.getFirst().data().amount());
            assertEquals(dealerAddress, notoCashStates.getFirst().data().owner());
            var notoBondStates = notoBond.queryStates(notoSchema.id, null);
            assertEquals(1, notoBondStates.size());
            assertEquals("1000", notoBondStates.getFirst().data().amount());
            assertEquals(agentAddress, notoBondStates.getFirst().data().owner());

            // Validate bond tracker balance
            assertEquals("1000", bondTracker.balanceOf(csdRequester, agentAddress));

            // Add dealer to investor list
            investorList.addInvestor(csdRequester, dealerAddress);

            // Enter DvP and RvP requests (as if received by issuer)
            bondOrders.deliverOrder(csdRequester, isin, new CSDBondOrdersHelper.BondDetails(agent, dealer));
            var result = bondOrders.receiveOrder(csdRequester, isin, new CSDBondOrdersHelper.BondDetails(agent, dealer));
            var receipt = TestbedHelper.getTransactionReceipt(testbed, result.id());
            var penteReceipt = mapper.convertValue(receipt.domainReceipt(), TestbedHelper.PenteDomainReceipt.class);
            assertEquals(1, penteReceipt.receipt().logs().size());
            var penteEvent = penteReceipt.receipt().logs().getFirst();
            var decodedEvent = TestbedHelper.decodeEvent(testbed, penteEvent.topics(), penteEvent.data());
            assertEquals("OrderMatched(string)", decodedEvent.signature());

            // Prepare the distribution
            bondOrders.prepareDistribution(csdRequester, isin);
            bondOrders.approveDistribution(csdChecker, isin);

            // Prepare the bond transfer (requires 2 calls to prepare, as the Noto transaction spawns a Pente transaction to wrap it)
            var bondTransfer = notoBond.prepareTransfer(agent, dealer, 1000);
            assertEquals("private", bondTransfer.preparedTransaction().type());
            assertEquals("pente", bondTransfer.preparedTransaction().domain());
            assertEquals(csdInstance.address(), bondTransfer.preparedTransaction().to().toString());
            assertEquals(1, bondTransfer.preparedTransaction().abi().size());
            var bondTransfer2 = csdInstance.prepare(
                    bondTransfer.preparedTransaction().from(),
                    bondTransfer.preparedTransaction().abi().getFirst(),
                    bondTransfer.preparedTransaction().data()
            );
            assertEquals("public", bondTransfer2.preparedTransaction().type());
            var bondTransferMetadata = mapper.convertValue(bondTransfer2.preparedMetadata(), PenteHelper.PenteTransitionMetadata.class);

            // Prepare the payment transfer
            var paymentTransfer = notoCash.prepareTransfer(dealer, agent, 1000);
            assertEquals("public", paymentTransfer.preparedTransaction().type());
            var paymentMetadata = mapper.convertValue(paymentTransfer.preparedMetadata(), NotoHelper.NotoTransferMetadata.class);

            // Prepare the atomic operation
            var atom = atomFactory.create(csdRequester, List.of(
                    new AtomHelper.Operation(bondTransfer2.preparedTransaction().to(), bondTransferMetadata.transitionWithApproval().encodedCall()),
                    new AtomHelper.Operation(paymentTransfer.preparedTransaction().to(), paymentMetadata.transferWithApproval().encodedCall())
            ));
            assertNotNull(atom);

            // Dealer approves payment transfer
            notoCash.approveTransfer(
                    dealer,
                    paymentTransfer.inputStates(),
                    paymentTransfer.outputStates(),
                    paymentMetadata.approvalParams().data(),
                    atom.address().toString());

            // Agent approves bond transfer
            var txID = csdInstance.approveTransition(
                    csdRequester,
                    JsonHex.randomBytes32(),
                    atom.address(),
                    bondTransferMetadata.approvalParams().transitionHash(),
                    bondTransferMetadata.approvalParams().signatures());
            receipt = TestbedHelper.pollForReceipt(testbed, txID, 3000);
            assertNotNull(receipt);

            // Execute the atomic operation
            atom.execute(csdRequester);
        }
    }
}
