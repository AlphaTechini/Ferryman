// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

import {IERC20} from "../../src/interfaces/IERC20.sol";
import {ILockCallback} from "../../src/interfaces/infinity/ILockCallback.sol";
import {BalanceDelta, amount0, amount1} from "../../src/libraries/InfinityTypes.sol";

/// @dev Models the parts of PancakeSwap Infinity's Vault that Ferry In uses:
///      - lock calls lockAcquired on msg.sender and reverts with CurrencyNotSettled if any locker delta is nonzero;
///      - sync/settle credit what was paid since the last sync (ERC20) or msg.value (native), exactly like Vault._settle;
///      - take debits the caller and transfers out;
///      - accountAppBalanceDelta only accepts registered apps (the CLPoolManager).
contract MockVault {
    error CurrencyNotSettled();
    error NoLocker();
    error LockerAlreadySet(address locker);
    error SettleNonNativeCurrencyWithValue();
    error AppUnregistered(address app);
    error TransferFailed();

    address internal _locker;
    mapping(address => mapping(address => int256)) internal _deltas;
    address[] internal _touched;
    mapping(address => bool) internal _isTouched;

    address internal _reserveCurrency;
    uint256 internal _reserveBalance;
    mapping(address => bool) internal _apps;

    receive() external payable {}

    function registerApp(address app) external {
        _apps[app] = true;
    }

    function isAppRegistered(address app) external view returns (bool) {
        return _apps[app];
    }

    function getLocker() external view returns (address) {
        return _locker;
    }

    function currencyDelta(address settler, address currency) external view returns (int256) {
        return _deltas[settler][currency];
    }

    function lock(bytes calldata data) external returns (bytes memory result) {
        if (_locker != address(0)) revert LockerAlreadySet(_locker);
        _locker = msg.sender;

        result = ILockCallback(msg.sender).lockAcquired(data);

        for (uint256 i = 0; i < _touched.length; ++i) {
            address currency = _touched[i];
            if (_deltas[msg.sender][currency] != 0) revert CurrencyNotSettled();
            _isTouched[currency] = false;
        }
        delete _touched;
        _locker = address(0);
    }

    function accountAppBalanceDelta(address currency0, address currency1, BalanceDelta delta, address settler)
        external
    {
        if (!_apps[msg.sender]) revert AppUnregistered(msg.sender);
        _account(settler, currency0, amount0(delta));
        _account(settler, currency1, amount1(delta));
    }

    function sync(address currency) external {
        _reserveCurrency = currency;
        _reserveBalance = currency == address(0) ? 0 : IERC20(currency).balanceOf(address(this));
    }

    function settle() external payable returns (uint256 paid) {
        if (_locker == address(0)) revert NoLocker();
        address currency = _reserveCurrency;
        if (currency != address(0)) {
            if (msg.value > 0) revert SettleNonNativeCurrencyWithValue();
            paid = IERC20(currency).balanceOf(address(this)) - _reserveBalance;
            _reserveCurrency = address(0);
            _reserveBalance = 0;
        } else {
            paid = msg.value;
        }
        _account(msg.sender, currency, int256(paid));
    }

    function take(address currency, address to, uint256 amount) external {
        if (_locker == address(0)) revert NoLocker();
        _account(msg.sender, currency, -int256(amount));
        if (currency == address(0)) {
            (bool ok,) = to.call{value: amount}("");
            if (!ok) revert TransferFailed();
        } else {
            if (!IERC20(currency).transfer(to, amount)) revert TransferFailed();
        }
    }

    function _account(address settler, address currency, int256 amount) internal {
        if (amount == 0) return;
        if (!_isTouched[currency]) {
            _isTouched[currency] = true;
            _touched.push(currency);
        }
        _deltas[settler][currency] += amount;
    }
}
