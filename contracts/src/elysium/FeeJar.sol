// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

/// @notice Collects FerryIn fees in HYPE. Anyone can trigger release; funds only go to the immutable beneficiary.
contract FeeJar {
    error ZeroAddress();
    error TransferFailed();

    event Released(uint256 amount);

    address public immutable beneficiary;

    constructor(address beneficiary_) {
        if (beneficiary_ == address(0)) revert ZeroAddress();
        beneficiary = beneficiary_;
    }

    receive() external payable {}

    function release() external {
        uint256 amount = address(this).balance;
        (bool ok,) = beneficiary.call{value: amount}("");
        if (!ok) revert TransferFailed();
        emit Released(amount);
    }
}
