// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.20;

import {NotoSelfSubmit} from "./NotoSelfSubmit.sol";

contract NotoSelfSubmitFactory {
    function deploy(
        bytes32 transactionId,
        address notary,
        bytes memory data
    ) external {
        new NotoSelfSubmit(transactionId, address(this), notary, data);
    }
}