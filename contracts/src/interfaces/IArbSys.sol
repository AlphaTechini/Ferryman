// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

/// @notice Subset of Arbitrum's ArbSys precompile (address 0x64) used by ExitDesk.
interface IArbSys {
    function withdrawEth(address destination) external payable returns (uint256);
}
