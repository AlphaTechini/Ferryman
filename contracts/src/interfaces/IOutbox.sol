// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

/// @notice Subset of the Arbitrum Outbox used to execute a matured L2 -> L1 message on HyperEVM.
interface IOutbox {
    function executeTransaction(
        bytes32[] calldata proof,
        uint256 index,
        address l2Sender,
        address to,
        uint256 l2Block,
        uint256 l1Block,
        uint256 l2Timestamp,
        uint256 value,
        bytes calldata data
    ) external;
}
