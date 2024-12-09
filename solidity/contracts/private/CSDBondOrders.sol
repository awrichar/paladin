// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {Ownable} from "@openzeppelin/contracts/access/Ownable.sol";

contract CSDBondOrders is Ownable {
    address public bondTracker;
    mapping(string => BondDetails) internal toDeliver;
    mapping(string => BondDetails) internal toReceive;
    mapping(string => address) internal maker;
    mapping(string => address) internal checker;
    mapping(string => bool) internal distributed;

    struct BondDetails {
        string agent;
        string dealer;
    }

    event OrderMatched(string isin);

    constructor(address bondTracker_) Ownable(_msgSender()) {
        bondTracker = bondTracker_;
    }

    function deliverOrder(
        string calldata isin,
        BondDetails calldata details
    ) external onlyOwner {
        require(
            maker[isin] == address(0),
            "Distribution has already been prepared"
        );

        toDeliver[isin] = details;

        if (_checkMatch(isin)) {
            emit OrderMatched(isin);
        }
    }

    function receiveOrder(
        string calldata isin,
        BondDetails calldata details
    ) external onlyOwner {
        require(
            maker[isin] == address(0),
            "Distribution has already been prepared"
        );

        toReceive[isin] = details;

        if (_checkMatch(isin)) {
            emit OrderMatched(isin);
        }
    }

    function _stringMatch(
        string memory a,
        string memory b
    ) internal pure returns (bool) {
        return keccak256(abi.encodePacked(a)) == keccak256(abi.encodePacked(b));
    }

    function _checkMatch(string calldata isin) internal view returns (bool) {
        BondDetails memory d = toDeliver[isin];
        BondDetails memory r = toReceive[isin];
        return
            _stringMatch(d.agent, r.agent) && _stringMatch(d.dealer, r.dealer);
    }

    function prepareDistribution(string calldata isin) external {
        require(
            maker[isin] == address(0),
            "Distribution has already been prepared"
        );
        require(_checkMatch(isin), "Orders have not been matched");
        maker[isin] = _msgSender();
    }

    function approveDistribution(string calldata isin) external {
        require(
            maker[isin] != address(0),
            "Distribution has not been prepared"
        );
        require(
            checker[isin] == address(0),
            "Distribution has already been approved"
        );
        require(
            maker[isin] != _msgSender(),
            "Maker and checker cannot be the same"
        );
        checker[isin] = _msgSender();
    }

    function markDistributed(string calldata isin) external onlyOwner {
        require(
            checker[isin] != address(0),
            "Distribution has not been approved"
        );
        distributed[isin] = true;
    }
}
