package io.kaleido.paladin.pente.domain.helpers;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.kaleido.paladin.testbed.Testbed;
import io.kaleido.paladin.toolkit.JsonABI;
import io.kaleido.paladin.toolkit.JsonHex;

import java.io.IOException;
import java.util.HashMap;
import java.util.List;

public class TestbedHelper {
    private static final int POLL_INTERVAL_MS = 100;

    @JsonIgnoreProperties(ignoreUnknown = true)
    public record TransactionReceipt(
            @JsonProperty
            String id,
            @JsonProperty
            boolean success,
            @JsonProperty
            String transactionHash,
            @JsonProperty
            long blockNumber,
            @JsonProperty
            JsonNode domainReceipt
    ) {
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    public record PenteDomainReceipt(
            @JsonProperty
            PenteDomainReceiptDetails receipt
    ) {
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    public record PenteDomainReceiptDetails(
            @JsonProperty
            JsonHex.Address from,
            @JsonProperty
            JsonHex.Address to,
            @JsonProperty
            JsonHex.Address contractAddress,
            @JsonProperty
            List<PenteEVMEvent> logs
    ) {
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    public record PenteEVMEvent(
            @JsonProperty
            JsonHex.Address address,
            @JsonProperty
            List<JsonHex.Bytes32> topics,
            @JsonProperty
            JsonHex.Bytes data
    ) {
    }

    @JsonIgnoreProperties(ignoreUnknown = true)
    public record DecodedEvent(
            @JsonProperty
            String signature
    ) {
    }

    public static Testbed.TransactionResult getTransactionResult(HashMap<String, Object> res) {
        return new ObjectMapper().convertValue(res, Testbed.TransactionResult.class);
    }

    public static String sendTransaction(Testbed testbed, Testbed.TransactionInput input) throws IOException {
        return testbed.getRpcClient().request("ptx_sendTransaction", input);
    }

    public static TransactionReceipt getTransactionReceipt(Testbed testbed, String txID) throws IOException {
        var result = testbed.getRpcClient().request("ptx_getTransactionReceiptFull", txID);
        return new ObjectMapper().convertValue(result, TransactionReceipt.class);
    }

    public static TransactionReceipt pollForReceipt(Testbed testbed, String txID, int waitMs) throws IOException, InterruptedException {
        for (var i = 0; i < waitMs; i += POLL_INTERVAL_MS) {
            var receipt = getTransactionReceipt(testbed, txID);
            if (receipt != null) {
                return new ObjectMapper().convertValue(receipt, TransactionReceipt.class);
            }
            Thread.sleep(POLL_INTERVAL_MS);
        }
        return null;
    }

    public static void storeABI(Testbed testbed, JsonABI abi) throws IOException {
        testbed.getRpcClient().request("ptx_storeABI", abi);
    }

    public static DecodedEvent decodeEvent(Testbed testbed, List<JsonHex.Bytes32> topics, JsonHex.Bytes data) throws IOException {
        var event = testbed.getRpcClient().request("ptx_decodeEvent", topics, data, "");
        return new ObjectMapper().convertValue(event, DecodedEvent.class);
    }
}
