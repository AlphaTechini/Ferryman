// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

import {ExitParams, ExitParamsLib} from "../libraries/ExitParams.sol";
import {Receiver} from "./Receiver.sol";

/// @notice HyperEVM side of Ferry Out. Executes the Outbox claim for one exit and pays out the Receiver
///         in the same call, so the tip cannot be front-run by a separate claim transaction.
/// @dev Immutable and ownerless. The receiver address is a pure function of ExitParams, so the claim
///      calldata can only pay the receiver that the Elysium exit precomputed. The recipient is fixed by
///      the signed exit, never by the caller.
contract ExitFactory {
    error BadSelector();
    error WrongReceiver();
    error AlreadySwept();
    error NotClaimable();
    error NotSwept();
    error NothingToSweep();
    error TipTooHigh();
    error ReceiverMismatch();
    error Reentrancy();

    event Claimed(
        bytes32 indexed salt,
        address indexed receiver,
        address indexed recipient,
        uint256 paid,
        uint256 tip,
        address keeper
    );

    /// @dev Outbox.executeTransaction selector.
    bytes4 internal constant EXECUTE_TRANSACTION_SELECTOR = 0x08635a95;

    uint256 private constant _NOT_ENTERED = 1;
    uint256 private constant _ENTERED = 2;

    address public immutable OUTBOX;
    bytes32 public immutable receiverInitHash;

    uint256 private _status = _NOT_ENTERED;
    mapping(bytes32 => bool) private _swept;

    modifier nonReentrant() {
        if (_status == _ENTERED) revert Reentrancy();
        _status = _ENTERED;
        _;
        _status = _NOT_ENTERED;
    }

    constructor(address outbox) {
        OUTBOX = outbox;
        receiverInitHash = keccak256(abi.encodePacked(type(Receiver).creationCode, abi.encode(address(this))));
    }

    /// @notice Precomputed Receiver address for an exit.
    function receiverFor(ExitParams calldata p) external view returns (address) {
        return _receiverOf(p);
    }

    function isSwept(bytes32 salt) external view returns (bool) {
        return _swept[salt];
    }

    /// @notice Executes the Outbox claim for `p` (if still needed), deploys the Receiver, and pays out.
    /// @param outboxCalldata Ready-made `Outbox.executeTransaction` calldata whose `to` is the receiver.
    /// @return paid Amount sent to `p.recipient` (`p.amount - p.claimTip`).
    function claimAndSweep(bytes calldata outboxCalldata, ExitParams calldata p)
        external
        nonReentrant
        returns (uint256 paid)
    {
        if (p.claimTip > p.amount) revert TipTooHigh();

        // (a) The calldata must be an Outbox.executeTransaction call.
        if (outboxCalldata.length < 4 || bytes4(outboxCalldata[:4]) != EXECUTE_TRANSACTION_SELECTOR) {
            revert BadSelector();
        }

        // (b) The Outbox payout must target the receiver for these exact params.
        address receiver = _receiverOf(p);
        (,,, address to,,,,,) = abi.decode(
            outboxCalldata[4:], (bytes32[], uint256, address, address, uint256, uint256, uint256, uint256, bytes)
        );
        if (to != receiver) revert WrongReceiver();

        bytes32 exitSalt = ExitParamsLib.salt(p);

        // (c) Each exit is swept at most once.
        if (_swept[exitSalt]) revert AlreadySwept();

        // (d) Execute the claim. If it reverts (typically already executed by someone else), continue
        //     only when the payout is already sitting in the receiver.
        (bool ok,) = OUTBOX.call(outboxCalldata);
        if (!ok && receiver.balance < p.amount) revert NotClaimable();

        // (e) Deploy the receiver at its CREATE2 address if it does not exist yet.
        if (receiver.code.length == 0) {
            Receiver deployed = new Receiver{salt: exitSalt}(address(this));
            if (address(deployed) != receiver) revert ReceiverMismatch();
        }

        // (f) The full exit amount must be present before anything is paid.
        if (receiver.balance < p.amount) revert NotClaimable();
        _swept[exitSalt] = true;

        // (g) Pay the recipient the net amount and the keeper the tip. Zero transfers are skipped.
        paid = p.amount - p.claimTip;
        Receiver r = Receiver(payable(receiver));
        if (paid > 0) r.pull(p.recipient, paid);
        if (p.claimTip > 0) r.pull(msg.sender, p.claimTip);

        emit Claimed(exitSalt, receiver, p.recipient, paid, p.claimTip, msg.sender);
    }

    /// @notice Sends any HYPE that reached the receiver after the claim (stray or late funds) to the recipient.
    function sweep(ExitParams calldata p) external nonReentrant {
        if (!_swept[ExitParamsLib.salt(p)]) revert NotSwept();
        address receiver = _receiverOf(p);
        uint256 balance = receiver.balance;
        if (balance == 0) revert NothingToSweep();
        Receiver(payable(receiver)).pull(p.recipient, balance);
    }

    function _receiverOf(ExitParams memory p) internal view returns (address) {
        return ExitParamsLib.computeReceiver(address(this), ExitParamsLib.salt(p), receiverInitHash);
    }
}
