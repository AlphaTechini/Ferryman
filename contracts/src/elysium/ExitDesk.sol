// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

import {ExitParams, ExitParamsLib} from "../libraries/ExitParams.sol";
import {IArbSys} from "../interfaces/IArbSys.sol";

/// @notice Elysium side of Ferry Out. One transaction starts a HYPE exit to a precomputed HyperEVM receiver.
contract ExitDesk {
    error ZeroAmount();
    error TipTooHigh();
    error ZeroRecipient();

    event ExitIntent(bytes32 indexed salt, address indexed receiver, address indexed recipient, ExitParams params);

    address internal constant ARB_SYS = address(0x64);

    address public immutable exitFactory;
    bytes32 public immutable receiverInitHash;

    mapping(address => uint256) public nonces;

    constructor(address exitFactory_, bytes32 receiverInitHash_) {
        exitFactory = exitFactory_;
        receiverInitHash = receiverInitHash_;
    }

    /// @notice Params and receiver the next `exitHYPE` from `sender` would produce.
    function previewReceiver(address sender, address recipient, uint256 amount, uint256 claimTip, bool toCore)
        external
        view
        returns (ExitParams memory params, address receiver)
    {
        params = _params(sender, nonces[sender], recipient, amount, claimTip, toCore);
        receiver = ExitParamsLib.computeReceiver(exitFactory, ExitParamsLib.salt(params), receiverInitHash);
    }

    /// @notice Starts an exit of `msg.value` HYPE. The HyperEVM receiver is returned and emitted.
    function exitHYPE(address recipient, uint256 claimTip, bool toCore) external payable returns (address receiver) {
        if (msg.value == 0) revert ZeroAmount();
        if (claimTip > msg.value) revert TipTooHigh();
        if (recipient == address(0)) revert ZeroRecipient();

        uint256 nonce = nonces[msg.sender];
        nonces[msg.sender] = nonce + 1;

        ExitParams memory params = _params(msg.sender, nonce, recipient, msg.value, claimTip, toCore);
        bytes32 salt = ExitParamsLib.salt(params);
        receiver = ExitParamsLib.computeReceiver(exitFactory, salt, receiverInitHash);

        IArbSys(ARB_SYS).withdrawEth{value: msg.value}(receiver);
        emit ExitIntent(salt, receiver, recipient, params);
    }

    function _params(address sender, uint256 nonce, address recipient, uint256 amount, uint256 claimTip, bool toCore)
        internal
        view
        returns (ExitParams memory)
    {
        return ExitParams({
            chainId: block.chainid,
            desk: address(this),
            sender: sender,
            nonce: nonce,
            amount: amount,
            claimTip: claimTip,
            recipient: recipient,
            toCore: toCore
        });
    }
}
