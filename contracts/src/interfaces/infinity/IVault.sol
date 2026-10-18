// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

/// @notice Subset of PancakeSwap Infinity's Vault used by the swap adapter (addresses stand in for Currency).
/// @dev Vault address on Elysium testnet: 0x2B738A9A69b7a45C9dA6B519f6bE6efBfb912aec (unverified).
interface IVault {
    /// @notice Runs `ILockCallback(msg.sender).lockAcquired(data)`. Reverts unless every delta nets to zero.
    function lock(bytes calldata data) external returns (bytes memory);

    /// @notice Checkpoints the ERC20 balance (or zeroes the reserve for native) before a payment.
    function sync(address currency) external;

    /// @notice Credits the caller with what it paid since `sync`, or with msg.value for native.
    function settle() external payable returns (uint256 paid);

    /// @notice Debits the caller and sends `amount` of `currency` to `to`.
    function take(address currency, address to, uint256 amount) external;

    /// @notice True when `app` may account pool deltas (the CLPoolManager must be registered).
    function isAppRegistered(address app) external returns (bool);

    function currencyDelta(address settler, address currency) external view returns (int256);
}
