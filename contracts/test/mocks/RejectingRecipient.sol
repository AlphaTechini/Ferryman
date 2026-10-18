// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

/// @dev Refuses every ETH transfer. Used to exercise TransferFailed paths.
contract RejectingRecipient {
    receive() external payable {
        revert("NO_ETH");
    }
}
