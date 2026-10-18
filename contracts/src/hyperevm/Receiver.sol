// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

/// @notice Holds HYPE for one exit. Deployed by ExitFactory via CREATE2; only the factory can move funds out.
contract Receiver {
    error NotFactory();
    error TransferFailed();

    address public immutable factory;

    constructor(address factory_) {
        factory = factory_;
    }

    /// @notice Accepts the Outbox payout, including before this contract is deployed.
    receive() external payable {}

    function pull(address to, uint256 amount) external {
        if (msg.sender != factory) revert NotFactory();
        (bool ok,) = to.call{value: amount}("");
        if (!ok) revert TransferFailed();
    }
}
