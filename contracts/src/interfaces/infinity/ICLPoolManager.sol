// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

import {PoolKey, BalanceDelta} from "../../libraries/InfinityTypes.sol";

/// @notice Subset of PancakeSwap Infinity's CLPoolManager used by Ferry In.
interface ICLPoolManager {
    struct SwapParams {
        bool zeroForOne;
        int256 amountSpecified; // negative = exact input
        uint160 sqrtPriceLimitX96;
    }

    /// @dev `id` is the PoolId, which is a bytes32 user-defined type in infinity-core (same ABI).
    function getSlot0(bytes32 id)
        external
        view
        returns (uint160 sqrtPriceX96, int24 tick, uint24 protocolFee, uint24 lpFee);

    function swap(PoolKey memory key, SwapParams memory params, bytes calldata hookData)
        external
        returns (BalanceDelta delta);
}
