// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.20;

import {Context} from "@openzeppelin/contracts/utils/Context.sol";
import {IPenteExternalCall} from "../domains/interfaces/IPenteExternalCall.sol";
import {Atom, AtomFactory} from "../shared/Atom.sol";

/**
 * @title InterbankTransfer
 * @dev Directs a swap of tokenized deposits between two banks.
 *      Tokenized deposits will be burned from the paying client, and tokenized
 *      deposits will be minted to the receiving client. The liability between
 *      the two banks will be settled in central bank digital currency (CBDC).
 */
contract InterbankTransfer is Context, IPenteExternalCall {
    enum Status {
        PENDING,
        EXPORTED,
        CANCELLED
    }

    struct Participant {
        address token;
        address wallet;
        address cbdcWallet;
    }

    struct StateEncoded {
        bytes id;
        string domain;
        bytes32 schema;
        address contractAddress;
        bytes data;
    }

    struct StateData {
        StateEncoded[] inputs;
        StateEncoded[] outputs;
    }

    struct PreparedTransaction {
        StateData states;
        bytes call;
    }

    struct Transfer {
        Status status;
        uint256 value;
        PreparedTransaction senderPrepared;
        PreparedTransaction cbdcPrepared;
        PreparedTransaction receiverPrepared;
    }

    address public cbdc;
    Participant public sender;
    Participant public receiver;
    address public atomFactory;

    mapping(uint256 => Transfer) public transfers;
    uint256 internal _index;

    event TransferStarted(
        uint256 indexed index,
        address indexed sender,
        address indexed receiver,
        uint256 value
    );

    event TransferAccepted(
        uint256 indexed index,
        address indexed sender,
        address indexed receiver,
        uint256 value
    );

    /**
     * Constructor
     * @param cbdc_ The address of the CBDC contract
     * @param sender_ The sender's participant information
     * @param receiver_ The receiver's participant information
     * @param atomFactory_ The address of the AtomFactory contract
     */
    constructor(
        address cbdc_,
        Participant memory sender_,
        Participant memory receiver_,
        address atomFactory_
    ) {
        cbdc = cbdc_;
        sender = sender_;
        receiver = receiver_;
        atomFactory = atomFactory_;
    }

    /**
     * Propose a new transfer.
     * Prepares the sender's deposit (burn) and CBDC (transfer) transactions.
     */
    function startTransfer(
        uint256 value,
        StateData calldata senderStates,
        bytes calldata senderCall,
        StateData calldata cbdcStates,
        bytes calldata cbdcCall
    ) external {
        require(_msgSender() == sender.wallet, "Invalid sender address");
        _index++;
        Transfer storage transfer = transfers[_index];
        transfer.status = Status.PENDING;
        transfer.value = value;
        transfer.senderPrepared.states = senderStates;
        transfer.senderPrepared.call = senderCall;
        transfer.cbdcPrepared.states = cbdcStates;
        transfer.cbdcPrepared.call = cbdcCall;
        emit TransferStarted(_index, sender.wallet, receiver.wallet, value);
    }

    /**
     * Accept a transfer.
     * Prepares the receiver's deposit (mint) transaction.
     */
    function acceptTransfer(
        uint256 index,
        StateData calldata receiverStates,
        bytes calldata receiverCall
    ) external {
        require(_msgSender() == receiver.wallet, "Invalid receiver address");
        Transfer storage transfer = transfers[index];
        require(transfer.status == Status.PENDING, "Transfer is not pending");
        transfer.receiverPrepared.states = receiverStates;
        transfer.receiverPrepared.call = receiverCall;
        emit TransferAccepted(
            index,
            sender.wallet,
            receiver.wallet,
            transfer.value
        );
    }

    /**
     * Deploy the Atom contract to the base ledger, which can be used to execute the transaction.
     */
    function export(uint256 index) external {
        Transfer storage transfer = transfers[index];
        require(transfer.status == Status.PENDING, "Transfer is not pending");
        require(
            _msgSender() == sender.wallet || _msgSender() == receiver.wallet,
            "May only be called by sender or receiver"
        );
        require(
            transfer.senderPrepared.call.length > 0 &&
                transfer.cbdcPrepared.call.length > 0,
            "Sender transfer has not been prepared"
        );
        require(
            transfer.receiverPrepared.call.length > 0,
            "Receiver transfer has not been prepared"
        );
        transfer.status = Status.EXPORTED;

        Atom.Operation[] memory operations = new Atom.Operation[](3);
        operations[0] = Atom.Operation(cbdc, transfer.cbdcPrepared.call);
        operations[1] = Atom.Operation(
            sender.token,
            transfer.senderPrepared.call
        );
        operations[2] = Atom.Operation(
            receiver.token,
            transfer.receiverPrepared.call
        );
        emit PenteExternalCall(
            atomFactory,
            abi.encodeCall(AtomFactory.create, operations)
        );
    }

    function cancel(uint256 index) external {
        Transfer storage transfer = transfers[index];
        require(transfer.status == Status.PENDING, "Transfer is not pending");
        require(
            _msgSender() == sender.wallet || _msgSender() == receiver.wallet,
            "May only be called by sender or receiver"
        );
        transfer.status = Status.CANCELLED;
    }
}
