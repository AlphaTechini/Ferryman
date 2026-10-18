// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

/// @dev Stands in for ArbSys at 0x64 (deployed with vm.etch). Records the last withdrawal and keeps the value.
contract MockArbSys {
    address public lastDestination;
    uint256 public lastValue;
    uint256 public withdrawals;

    function withdrawEth(address destination) external payable returns (uint256 id) {
        lastDestination = destination;
        lastValue = msg.value;
        id = withdrawals;
        withdrawals += 1;
    }
}
