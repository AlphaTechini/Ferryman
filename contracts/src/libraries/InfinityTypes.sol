// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

/// @notice Pool identity for PancakeSwap Infinity CL pools, trimmed to plain addresses.
/// @dev Field order matches infinity-core's PoolKey, so abi.encode(key) and the ABI tuple are identical.
///      Currencies are addresses, with address(0) meaning native HYPE.
struct PoolKey {
    address currency0; // lower address (sorted numerically)
    address currency1; // higher address
    address hooks;
    address poolManager; // CLPoolManager
    uint24 fee;
    bytes32 parameters; // hooks bitmap in bits [0,16), tickSpacing in bits [16,40)
}

/// @dev amount0 is the upper 128 bits, amount1 the lower 128 bits (both int128).
///      Negative means the caller owes the pool, positive means the pool owes the caller.
type BalanceDelta is int256;

using {amount0, amount1} for BalanceDelta global;

function amount0(BalanceDelta delta) pure returns (int128) {
    return int128(BalanceDelta.unwrap(delta) >> 128);
}

function amount1(BalanceDelta delta) pure returns (int128) {
    return int128(BalanceDelta.unwrap(delta));
}

/// @dev Packs two int128 values the same way infinity-core does.
function toBalanceDelta(int128 a0, int128 a1) pure returns (BalanceDelta) {
    // The low word is taken as unsigned so that adding it does not borrow from the upper word.
    // forge-lint: disable-next-line(unsafe-typecast)
    int256 low = int256(uint256(uint128(a1)));
    return BalanceDelta.wrap((int256(a0) << 128) + low);
}

/// @notice Pool id: keccak256 over the six 32-byte words of the key (same as PoolIdLibrary.toId).
function poolIdOf(PoolKey memory key) pure returns (bytes32) {
    return keccak256(abi.encode(key));
}
