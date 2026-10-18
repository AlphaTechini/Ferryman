// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

import {PoolKey, poolIdOf} from "../libraries/InfinityTypes.sol";
import {TokenTransfers} from "../libraries/TokenTransfers.sol";
import {ICLPoolManager} from "../interfaces/infinity/ICLPoolManager.sol";
import {IWHYPE} from "../interfaces/IWHYPE.sol";
import {InfinitySwapAdapter} from "./InfinitySwapAdapter.sol";

/// @notice Elysium side of Ferry In. Receives HYPE (usually via a HyperEVM retryable) and buys an Ascend token.
/// @dev Immutable and ownerless. buy() never reverts after it receives value: a failed swap refunds the
///      full amount to the recipient on Elysium, so no ticket is stranded.
///      Route: HYPE -> quote currency (fixed HYPE route, set at deploy) -> token (Ascend pool, registered permissionlessly).
contract FerryIn {
    error ZeroAddress();
    error ZeroValue();
    error NoRoutes();
    error BadRoute();
    error DuplicateRoute();
    error UnknownPoolManager(address poolManager);
    error NotAscendPool();
    error UnsupportedQuote();
    error AlreadyRegistered();
    error PoolNotInitialized();
    error NotRegistered();
    error NothingOwed();
    error TransferFailed();
    error UnexpectedValue();
    error Reentrancy();

    event TokenRegistered(address indexed token, address indexed quoteCurrency, PoolKey key);
    event Bought(address indexed token, address indexed recipient, uint256 hypeIn, uint256 fee, uint256 amountOut);
    event Refunded(address indexed token, address indexed recipient, uint256 hypeIn, bytes reason);
    event Owed(address indexed to, uint256 amount);
    event OwedClaimed(address indexed to, uint256 amount);

    /// @notice Fee in basis points of hypeIn, sent to FeeJar on a successful buy.
    uint256 public constant FEE_BPS = 30;

    InfinitySwapAdapter public immutable ADAPTER;
    address public immutable FEE_JAR;
    address public immutable LAUNCHPAD; // AscendLaunchpad; the only hooks accepted for token pools
    address public immutable WHYPE;
    address public immutable POOL_MANAGER; // CLPoolManager shared by every route and token pool

    uint256 private constant _NOT_ENTERED = 1;
    uint256 private constant _ENTERED = 2;
    uint256 private _status = _NOT_ENTERED;

    /// @dev quote currency => HYPE route pool (HYPE native or WHYPE on one side, quote on the other).
    mapping(address => PoolKey) private _routes;
    mapping(address => bool) private _isQuote;
    /// @dev token => registered Ascend pool (token on one side, a quote currency on the other).
    mapping(address => PoolKey) private _keys;
    mapping(address => bool) private _registered;
    /// @notice HYPE owed to an address when neither a native nor a WHYPE payout could be delivered.
    mapping(address => uint256) public owed;

    modifier nonReentrant() {
        if (_status == _ENTERED) revert Reentrancy();
        _status = _ENTERED;
        _;
        _status = _NOT_ENTERED;
    }

    /// @param adapter InfinitySwapAdapter on Elysium.
    /// @param feeJar FeeJar that collects the buy fee.
    /// @param launchpad AscendLaunchpad address (the hooks of every token pool).
    /// @param whype WHYPE on Elysium (or WHYPE9).
    /// @param hypeRoutes One pool per supported quote currency, from HYPE (native or WHYPE) to that quote.
    ///        All routes must share one poolManager and be initialized.
    constructor(address adapter, address feeJar, address launchpad, address whype, PoolKey[] memory hypeRoutes) {
        if (adapter == address(0) || feeJar == address(0) || launchpad == address(0) || whype == address(0)) {
            revert ZeroAddress();
        }
        if (adapter.code.length == 0) revert ZeroAddress();
        if (hypeRoutes.length == 0) revert NoRoutes();

        ADAPTER = InfinitySwapAdapter(adapter);
        FEE_JAR = feeJar;
        LAUNCHPAD = launchpad;
        WHYPE = whype;
        address poolManager = hypeRoutes[0].poolManager;
        POOL_MANAGER = poolManager;

        for (uint256 i = 0; i < hypeRoutes.length; ++i) {
            PoolKey memory route = hypeRoutes[i];
            if (route.poolManager != poolManager) revert UnknownPoolManager(route.poolManager);

            bool hype0 = _isHype(route.currency0, whype);
            bool hype1 = _isHype(route.currency1, whype);
            if (hype0 == hype1) revert BadRoute(); // exactly one side must be HYPE
            address quoteCurrency = hype0 ? route.currency1 : route.currency0;
            if (_isQuote[quoteCurrency]) revert DuplicateRoute();

            (uint160 sqrtPriceX96,,,) = ICLPoolManager(poolManager).getSlot0(poolIdOf(route));
            if (sqrtPriceX96 == 0) revert PoolNotInitialized();

            _routes[quoteCurrency] = route;
            _isQuote[quoteCurrency] = true;
        }
    }

    /// @notice Accepts native HYPE only from WHYPE (during a withdraw in the refund fallback).
    receive() external payable {
        if (msg.sender != WHYPE) revert UnexpectedValue();
    }

    /// @notice Registers an Ascend token. Anyone can call it. The first registration for a token wins.
    /// @dev Requires an Ascend pool (hooks == LAUNCHPAD) on the trusted CLPoolManager, with one side a supported quote currency
    ///      and the other side the token. The pool must be initialized.
    function registerToken(PoolKey calldata key) external {
        if (key.hooks != LAUNCHPAD) revert NotAscendPool();
        if (key.poolManager != POOL_MANAGER) revert UnknownPoolManager(key.poolManager);

        bool quote0 = _isQuote[key.currency0];
        bool quote1 = _isQuote[key.currency1];
        if (quote0 == quote1) revert UnsupportedQuote();
        (address token, address quoteCurrency) =
            quote0 ? (key.currency1, key.currency0) : (key.currency0, key.currency1);
        if (token == address(0) || _isHype(token, WHYPE)) revert UnsupportedQuote();
        if (_registered[token]) revert AlreadyRegistered();

        (uint160 sqrtPriceX96,,,) = ICLPoolManager(key.poolManager).getSlot0(poolIdOf(key));
        if (sqrtPriceX96 == 0) revert PoolNotInitialized();

        _keys[token] = key;
        _registered[token] = true;
        emit TokenRegistered(token, quoteCurrency, key);
    }

    /// @notice Buys `token` with msg.value HYPE and sends it to `recipient` (msg.sender if zero).
    /// @dev The fee goes to FeeJar. Any failure refunds the full msg.value to the recipient. Never reverts after value is received.
    /// @param minOut Minimum token output. Below it the buy is refunded.
    function buy(address token, address recipient, uint256 minOut) external payable nonReentrant {
        uint256 hypeIn = msg.value;
        if (hypeIn == 0) revert ZeroValue();
        if (recipient == address(0)) recipient = msg.sender;

        if (!_registered[token]) {
            _refund(token, recipient, hypeIn, abi.encodeWithSelector(NotRegistered.selector));
            return;
        }

        uint256 fee = hypeIn * FEE_BPS / 10_000;
        uint256 swapIn = hypeIn - fee;

        try ADAPTER.swapExactIn{value: swapIn}(_path(token), address(0), swapIn, minOut, recipient) returns (
            uint256 amountOut
        ) {
            if (fee > 0) _payFee(fee, recipient);
            emit Bought(token, recipient, hypeIn, fee, amountOut);
        } catch (bytes memory reason) {
            _refund(token, recipient, hypeIn, reason);
        }
    }

    /// @notice Quotes `buy(token, ., .)` for `hypeIn` (fee taken first). Meant for eth_call. Reverts if the token is not registered.
    function quote(address token, uint256 hypeIn) external returns (uint256 amountOut) {
        if (!_registered[token]) revert NotRegistered();
        uint256 swapIn = hypeIn - hypeIn * FEE_BPS / 10_000;
        return ADAPTER.quoteExactIn(_path(token), address(0), swapIn);
    }

    /// @notice Claims HYPE that could not be delivered by a refund or fee payout.
    function claimOwed() external nonReentrant {
        uint256 amount = owed[msg.sender];
        if (amount == 0) revert NothingOwed();
        owed[msg.sender] = 0;
        (bool ok,) = msg.sender.call{value: amount}("");
        if (!ok) revert TransferFailed();
        emit OwedClaimed(msg.sender, amount);
    }

    function poolKeyOf(address token) external view returns (PoolKey memory) {
        return _keys[token];
    }

    function isRegistered(address token) external view returns (bool) {
        return _registered[token];
    }

    function isQuoteCurrency(address currency) external view returns (bool) {
        return _isQuote[currency];
    }

    /// @notice HYPE -> quote route for a supported quote currency.
    function routeOf(address quoteCurrency) external view returns (PoolKey memory) {
        return _routes[quoteCurrency];
    }

    function _path(address token) internal view returns (PoolKey[] memory path) {
        PoolKey memory key = _keys[token];
        address quoteCurrency = key.currency0 == token ? key.currency1 : key.currency0;
        path = new PoolKey[](2);
        path[0] = _routes[quoteCurrency];
        path[1] = key;
    }

    function _refund(address token, address recipient, uint256 hypeIn, bytes memory reason) internal {
        _payHype(recipient, hypeIn);
        emit Refunded(token, recipient, hypeIn, reason);
    }

    function _payFee(uint256 fee, address recipient) internal {
        (bool ok,) = FEE_JAR.call{value: fee}("");
        if (!ok) _payHype(recipient, fee);
    }

    /// @dev Pays HYPE in order: native, then WHYPE, then the owed mapping. Never reverts.
    function _payHype(address to, uint256 amount) internal {
        (bool ok,) = to.call{value: amount}("");
        if (ok) return;
        if (_wrapAndSend(to, amount)) return;
        owed[to] += amount;
        emit Owed(to, amount);
    }

    /// @dev Wraps and sends WHYPE. If the transfer fails, unwraps back so the caller can fall back to owed.
    function _wrapAndSend(address to, uint256 amount) internal returns (bool) {
        (bool ok,) = WHYPE.call{value: amount}(abi.encodeCall(IWHYPE.deposit, ()));
        if (!ok) return false;
        if (TokenTransfers.tryTransfer(WHYPE, to, amount)) return true;
        (ok,) = WHYPE.call(abi.encodeCall(IWHYPE.withdraw, (amount)));
        return false;
    }

    function _isHype(address currency, address whype) internal pure returns (bool) {
        return currency == address(0) || currency == whype;
    }
}
