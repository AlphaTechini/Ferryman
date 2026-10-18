// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

import {IERC20} from "../interfaces/IERC20.sol";

/// @notice Low-level ERC20 transfers that accept tokens which return nothing or return true.
/// @dev Returns false instead of reverting so callers choose their own failure path.
library TokenTransfers {
    function tryTransfer(address token, address to, uint256 amount) internal returns (bool) {
        return _call(token, abi.encodeCall(IERC20.transfer, (to, amount)));
    }

    function tryTransferFrom(address token, address from, address to, uint256 amount) internal returns (bool) {
        return _call(token, abi.encodeCall(IERC20.transferFrom, (from, to, amount)));
    }

    function _call(address token, bytes memory data) private returns (bool) {
        (bool ok, bytes memory ret) = token.call(data);
        return ok && (ret.length == 0 || (ret.length == 32 && abi.decode(ret, (uint256)) == 1));
    }
}
