// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

/// @notice Wrapped HYPE, WETH9-style. Assumes deposit() and withdraw(uint256) exist (see docs, unverified on Elysium).
interface IWHYPE {
    function deposit() external payable;
    function withdraw(uint256 wad) external;
}
