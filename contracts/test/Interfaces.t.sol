// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

import {Test} from "forge-std/Test.sol";
import {IArbSys} from "../src/interfaces/IArbSys.sol";
import {IOutbox} from "../src/interfaces/IOutbox.sol";

contract InterfacesTest is Test {
    function test_ArbSysWithdrawEthSelector() public pure {
        assertEq(IArbSys.withdrawEth.selector, bytes4(keccak256("withdrawEth(address)")));
    }

    function test_OutboxSelectorIsExecuteTransaction() public pure {
        assertEq(
            IOutbox.executeTransaction.selector,
            bytes4(
                keccak256("executeTransaction(bytes32[],uint256,address,address,uint256,uint256,uint256,uint256,bytes)")
            )
        );
    }
}
