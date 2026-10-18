// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

import {IVault} from "../interfaces/infinity/IVault.sol";
import {ICLPoolManager} from "../interfaces/infinity/ICLPoolManager.sol";
import {ILockCallback} from "../interfaces/infinity/ILockCallback.sol";
import {IWHYPE} from "../interfaces/IWHYPE.sol";
import {PoolKey, BalanceDelta, amount0, amount1} from "../libraries/InfinityTypes.sol";
import {TokenTransfers} from "../libraries/TokenTransfers.sol";

/// @notice Exact-input swaps through PancakeSwap Infinity CL pools, one Vault lock per call.
/// @dev Immutable and ownerless. Ferryman's own adapter, not Ascend's Swapper.
///      - Every hop must use a CLPoolManager registered with the Vault, so a caller cannot fake a pool.
///      - Hops chain inside one lock. The intermediate currency nets to zero, so only the first input is
///        paid and only the last output is taken.
///      - A hop that does not spend the full input reverts (PartialFill), so no input is left behind.
///      - Native HYPE into a pool that trades WHYPE is wrapped before the lock.
contract InfinitySwapAdapter is ILockCallback {
    error NotVault();
    error BadPath();
    error ZeroAmount();
    error AmountTooLarge();
    error ZeroRecipient();
    error NativeValueMismatch();
    error UnexpectedValue();
    error TransferFailed();
    error UnknownPoolManager(address poolManager);
    error PartialFill(uint256 spent, uint256 amountIn);
    error ZeroOutput();
    error InsufficientOutput(uint256 amountOut, uint256 minOut);
    error QuoteResult(uint256 amountOut);
    error QuoteFailed();

    /// @dev TickMath bounds from infinity-core. A limit one unit inside each bound is the "no limit" value.
    uint160 internal constant MIN_SQRT_RATIO = 4295128739;
    uint160 internal constant MAX_SQRT_RATIO = 1461446703485210103287273052203988822378723970342;

    struct SwapData {
        PoolKey[] path;
        address currencyIn; // effective input currency after the HYPE to WHYPE mapping
        uint256 amountIn;
        uint256 minOut;
        address recipient;
        bool quoteOnly;
    }

    IVault public immutable VAULT;
    address public immutable WHYPE;

    constructor(address vault, address whype) {
        VAULT = IVault(vault);
        WHYPE = whype;
    }

    /// @notice Swaps `amountIn` of `tokenIn` along `path` and sends the final output to `recipient`.
    /// @param tokenIn address(0) for native HYPE (msg.value must equal amountIn), otherwise an ERC20 pulled from msg.sender.
    /// @return amountOut Realized output of the last hop.
    function swapExactIn(PoolKey[] calldata path, address tokenIn, uint256 amountIn, uint256 minOut, address recipient)
        external
        payable
        returns (uint256 amountOut)
    {
        if (amountIn == 0) revert ZeroAmount();
        if (amountIn > uint128(type(int128).max)) revert AmountTooLarge();
        if (recipient == address(0)) revert ZeroRecipient();
        if (path.length == 0) revert BadPath();

        address currencyIn = _effectiveCurrencyIn(path[0], tokenIn);
        if (tokenIn == address(0)) {
            if (msg.value != amountIn) revert NativeValueMismatch();
            if (currencyIn != address(0)) IWHYPE(WHYPE).deposit{value: amountIn}();
        } else {
            if (msg.value != 0) revert UnexpectedValue();
            if (!TokenTransfers.tryTransferFrom(tokenIn, msg.sender, address(this), amountIn)) {
                revert TransferFailed();
            }
        }

        bytes memory data = abi.encode(
            SwapData({
                path: path,
                currencyIn: currencyIn,
                amountIn: amountIn,
                minOut: minOut,
                recipient: recipient,
                quoteOnly: false
            })
        );
        amountOut = abi.decode(VAULT.lock(data), (uint256));
    }

    /// @notice Same route as swapExactIn, but always reverts with the output. Meant for eth_call through a wrapper.
    /// @dev Pays nothing and moves nothing. The revert rolls back every swap, and this function returns the decoded amount.
    function quoteExactIn(PoolKey[] calldata path, address tokenIn, uint256 amountIn)
        external
        returns (uint256 amountOut)
    {
        if (amountIn == 0) revert ZeroAmount();
        if (amountIn > uint128(type(int128).max)) revert AmountTooLarge();
        if (path.length == 0) revert BadPath();

        bytes memory data = abi.encode(
            SwapData({
                path: path,
                currencyIn: _effectiveCurrencyIn(path[0], tokenIn),
                amountIn: amountIn,
                minOut: 0,
                recipient: address(0),
                quoteOnly: true
            })
        );
        (bool ok, bytes memory ret) = address(VAULT).call(abi.encodeCall(IVault.lock, (data)));
        if (ok) revert QuoteFailed(); // lockAcquired always reverts in quote mode
        // forge-lint: disable-next-line(unsafe-typecast)
        if (ret.length == 36 && bytes4(ret) == QuoteResult.selector) {
            assembly ("memory-safe") {
                amountOut := mload(add(ret, 36))
            }
            return amountOut;
        }
        // Any other failure (paused pool, bad path, unknown pool manager) is bubbled up unchanged.
        assembly ("memory-safe") {
            revert(add(ret, 32), mload(ret))
        }
    }

    /// @inheritdoc ILockCallback
    function lockAcquired(bytes calldata data) external returns (bytes memory) {
        if (msg.sender != address(VAULT)) revert NotVault();
        SwapData memory s = abi.decode(data, (SwapData));

        address currency = s.currencyIn;
        uint256 amount = s.amountIn;
        for (uint256 i = 0; i < s.path.length; ++i) {
            (amount, currency) = _swapExactIn(s.path[i], currency, amount);
        }
        // `currency` is now the output currency and `amount` the realized output.

        if (s.quoteOnly) revert QuoteResult(amount);
        if (amount < s.minOut) revert InsufficientOutput(amount, s.minOut);

        _settle(s.currencyIn, s.amountIn);
        VAULT.take(currency, s.recipient, amount);
        return abi.encode(amount);
    }

    /// @dev One exact-input hop. Returns the realized output and its currency.
    function _swapExactIn(PoolKey memory key, address currencyIn, uint256 amountIn)
        internal
        returns (uint256 amountOut, address currencyOut)
    {
        if (!VAULT.isAppRegistered(key.poolManager)) revert UnknownPoolManager(key.poolManager);

        bool zeroForOne;
        if (currencyIn == key.currency0) {
            zeroForOne = true;
        } else if (currencyIn == key.currency1) {
            zeroForOne = false;
        } else {
            revert BadPath();
        }

        BalanceDelta delta = ICLPoolManager(key.poolManager)
            .swap(
                key,
                ICLPoolManager.SwapParams({
                zeroForOne: zeroForOne,
                // forge-lint: disable-next-line(unsafe-typecast)
                amountSpecified: -int256(amountIn), // amountIn <= int128.max, checked by the caller
                sqrtPriceLimitX96: zeroForOne ? MIN_SQRT_RATIO + 1 : MAX_SQRT_RATIO - 1
            }),
                ""
            );

        int256 inDelta = zeroForOne ? amount0(delta) : amount1(delta);
        int256 outDelta = zeroForOne ? amount1(delta) : amount0(delta);
        // forge-lint: disable-start(unsafe-typecast)
        if (inDelta != -int256(amountIn)) {
            revert PartialFill(inDelta < 0 ? uint256(-inDelta) : 0, amountIn);
        }
        if (outDelta <= 0) revert ZeroOutput();

        amountOut = uint256(outDelta);
        // forge-lint: disable-end(unsafe-typecast)
        currencyOut = zeroForOne ? key.currency1 : key.currency0;
    }

    /// @dev Pays `amount` of `currency` to the Vault: sync, then transfer (ERC20) or send value (native), then settle.
    function _settle(address currency, uint256 amount) internal {
        VAULT.sync(currency);
        if (currency == address(0)) {
            VAULT.settle{value: amount}();
        } else {
            if (!TokenTransfers.tryTransfer(currency, address(VAULT), amount)) revert TransferFailed();
            VAULT.settle();
        }
    }

    /// @dev Native HYPE into a pool that trades WHYPE becomes WHYPE. Everything else keeps its currency.
    function _effectiveCurrencyIn(PoolKey calldata first, address tokenIn) internal view returns (address) {
        if (tokenIn != address(0)) return tokenIn;
        if (first.currency0 == address(0) || first.currency1 == address(0)) return address(0);
        return WHYPE;
    }
}
