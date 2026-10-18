// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

import {FerryIn} from "../../src/elysium/FerryIn.sol";

/// @dev A recipient that refuses HYPE until `accept` is set. Used to exercise the refund fallbacks.
contract ToggleRecipient {
    bool public accept;

    function setAccept(bool value) external {
        accept = value;
    }

    function claim(FerryIn ferry) external {
        ferry.claimOwed();
    }

    receive() external payable {
        require(accept, "REJECTED");
    }
}
