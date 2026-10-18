// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

import {PoolKey, BalanceDelta, toBalanceDelta, poolIdOf} from "../../src/libraries/InfinityTypes.sol";
import {ICLPoolManager} from "../../src/interfaces/infinity/ICLPoolManager.sol";
import {MockVault} from "./MockVault.sol";

/// @dev Constant-rate CL pool manager. A pool's rate is currency1 per currency0, scaled by 1e18.
///      Exact-input swaps only. Current sqrt price is fixed at 2^96, so the limit checks match infinity-core.
contract MockCLPoolManager {
    error PoolNotInitialized();
    error PoolPaused();
    error SwapAmountCannotBeZero();
    error ExactOutputNotSupported();
    error InvalidSqrtPriceLimit();

    uint160 internal constant MIN_SQRT_RATIO = 4295128739;
    uint160 internal constant MAX_SQRT_RATIO = 1461446703485210103287273052203988822378723970342;
    uint160 internal constant CURRENT_SQRT_PRICE = 2 ** 96;

    struct Pool {
        bool initialized;
        bool paused;
        uint256 rate; // currency1 per currency0, 1e18 scale
        uint256 maxIn; // if nonzero, a swap spends at most this much (partial fill)
    }

    MockVault public immutable VAULT;
    mapping(bytes32 => Pool) public pools;

    constructor(MockVault vault) {
        VAULT = vault;
    }

    function initialize(PoolKey memory key, uint256 rate) external returns (bytes32 id) {
        id = poolIdOf(key);
        pools[id].initialized = true;
        pools[id].rate = rate;
    }

    function setRate(PoolKey memory key, uint256 rate) external {
        pools[poolIdOf(key)].rate = rate;
    }

    function setPaused(PoolKey memory key, bool paused) external {
        pools[poolIdOf(key)].paused = paused;
    }

    function setMaxIn(PoolKey memory key, uint256 maxIn) external {
        pools[poolIdOf(key)].maxIn = maxIn;
    }

    function getSlot0(bytes32 id) external view returns (uint160 sqrtPriceX96, int24, uint24, uint24) {
        if (pools[id].initialized) sqrtPriceX96 = CURRENT_SQRT_PRICE;
    }

    function swap(PoolKey memory key, ICLPoolManager.SwapParams memory params, bytes calldata)
        external
        returns (BalanceDelta delta)
    {
        Pool storage pool = pools[poolIdOf(key)];
        if (!pool.initialized) revert PoolNotInitialized();
        if (pool.paused) revert PoolPaused();
        if (params.amountSpecified == 0) revert SwapAmountCannotBeZero();
        if (params.amountSpecified > 0) revert ExactOutputNotSupported();

        if (params.zeroForOne) {
            if (params.sqrtPriceLimitX96 <= MIN_SQRT_RATIO || params.sqrtPriceLimitX96 >= CURRENT_SQRT_PRICE) {
                revert InvalidSqrtPriceLimit();
            }
        } else {
            if (params.sqrtPriceLimitX96 <= CURRENT_SQRT_PRICE || params.sqrtPriceLimitX96 >= MAX_SQRT_RATIO) {
                revert InvalidSqrtPriceLimit();
            }
        }

        uint256 requested = uint256(-params.amountSpecified);
        uint256 filled = pool.maxIn != 0 && requested > pool.maxIn ? pool.maxIn : requested;
        uint256 out = params.zeroForOne ? filled * pool.rate / 1e18 : filled * 1e18 / pool.rate;

        // forge-lint: disable-next-line(unsafe-typecast)
        int128 spent = -int128(uint128(filled));
        // forge-lint: disable-next-line(unsafe-typecast)
        int128 received = int128(uint128(out));
        delta = params.zeroForOne ? toBalanceDelta(spent, received) : toBalanceDelta(received, spent);
        VAULT.accountAppBalanceDelta(key.currency0, key.currency1, delta, msg.sender);
    }
}
