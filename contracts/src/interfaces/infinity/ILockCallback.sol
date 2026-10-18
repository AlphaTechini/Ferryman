// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

/// @notice Callback the Infinity Vault makes to msg.sender of `lock`.
interface ILockCallback {
    function lockAcquired(bytes calldata data) external returns (bytes memory);
}
