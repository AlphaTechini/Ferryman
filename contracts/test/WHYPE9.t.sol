// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

import {Test} from "forge-std/Test.sol";
import {WHYPE9} from "../src/elysium/WHYPE9.sol";

contract WHYPE9Test is Test {
    WHYPE9 internal w;
    address internal alice = makeAddr("alice");
    address internal bob = makeAddr("bob");

    function setUp() public {
        w = new WHYPE9();
        vm.deal(alice, 10 ether);
    }

    function test_Metadata() public view {
        assertEq(w.name(), "Wrapped HYPE");
        assertEq(w.symbol(), "WHYPE");
        assertEq(w.decimals(), 18);
    }

    function test_DepositAndReceiveWrap() public {
        vm.prank(alice);
        w.deposit{value: 1 ether}();
        assertEq(w.balanceOf(alice), 1 ether);

        vm.prank(alice);
        (bool ok,) = address(w).call{value: 2 ether}("");
        assertTrue(ok);
        assertEq(w.balanceOf(alice), 3 ether);
        assertEq(address(w).balance, 3 ether);
    }

    function test_WithdrawReturnsNative() public {
        vm.prank(alice);
        w.deposit{value: 2 ether}();
        vm.prank(alice);
        w.withdraw(1.5 ether);
        assertEq(w.balanceOf(alice), 0.5 ether);
        assertEq(alice.balance, 10 ether - 0.5 ether);
    }

    function test_WithdrawMoreThanBalanceReverts() public {
        vm.prank(alice);
        vm.expectRevert(WHYPE9.InsufficientBalance.selector);
        w.withdraw(1);
    }

    function test_TransferAndApprove() public {
        vm.startPrank(alice);
        w.deposit{value: 1 ether}();
        assertTrue(w.transfer(bob, 0.4 ether));
        assertTrue(w.approve(bob, 0.3 ether));
        vm.stopPrank();
        assertEq(w.balanceOf(bob), 0.4 ether);

        vm.prank(bob);
        assertTrue(w.transferFrom(alice, bob, 0.3 ether));
        assertEq(w.allowance(alice, bob), 0);
        assertEq(w.balanceOf(bob), 0.7 ether);

        vm.prank(bob);
        vm.expectRevert(WHYPE9.InsufficientAllowance.selector);
        w.transferFrom(alice, bob, 0.1 ether);
    }

    function test_MaxApprovalIsNotDecremented() public {
        vm.startPrank(alice);
        w.deposit{value: 1 ether}();
        assertTrue(w.approve(bob, type(uint256).max));
        vm.stopPrank();
        vm.prank(bob);
        w.transferFrom(alice, bob, 1 ether);
        assertEq(w.allowance(alice, bob), type(uint256).max);
    }

    function testFuzz_DepositWithdrawRoundTrip(uint96 amount) public {
        vm.deal(alice, amount);
        vm.startPrank(alice);
        w.deposit{value: amount}();
        w.withdraw(amount);
        vm.stopPrank();
        assertEq(w.balanceOf(alice), 0);
        assertEq(alice.balance, amount);
    }
}
