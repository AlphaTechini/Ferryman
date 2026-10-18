// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

import {FerryFixture} from "./FerryFixture.sol";
import {ExitParams, ExitParamsLib} from "../src/libraries/ExitParams.sol";
import {ExitDesk} from "../src/elysium/ExitDesk.sol";

contract ExitDeskTest is FerryFixture {
    function test_ExitSendsValueToPrecomputedReceiver() public {
        (ExitParams memory p, address receiver) = _previewOnElysium(alice, bob, 3 ether, 0.1 ether);
        _onElysium();
        vm.prank(alice);
        address got = desk.exitHYPE{value: 3 ether}(bob, 0.1 ether, false);

        assertEq(got, receiver);
        assertEq(arbSys.lastDestination(), receiver);
        assertEq(arbSys.lastValue(), 3 ether);
        assertEq(address(arbSys).balance, 3 ether);
        assertEq(p.amount, 3 ether);
        _onHyperEVM();
    }

    function test_ExitEmitsExitIntent() public {
        (ExitParams memory p, address receiver) = _previewOnElysium(alice, bob, 2 ether, 0);
        bytes32 salt = ExitParamsLib.salt(p);
        _onElysium();
        vm.expectEmit(true, true, true, true, address(desk));
        emit ExitDesk.ExitIntent(salt, receiver, bob, p);
        vm.prank(alice);
        desk.exitHYPE{value: 2 ether}(bob, 0, false);
        _onHyperEVM();
    }

    function test_PreviewMatchesFactoryReceiverFor() public {
        (ExitParams memory p, address receiver) = _previewOnElysium(alice, bob, 1 ether, 0.01 ether);
        _onHyperEVM();
        assertEq(factory.receiverFor(p), receiver);
    }

    function test_PreviewDoesNotConsumeNonce() public {
        _previewOnElysium(alice, bob, 1 ether, 0);
        _previewOnElysium(alice, bob, 1 ether, 0);
        assertEq(desk.nonces(alice), 0);
    }

    function test_NonceIncrementsPerSenderAndGivesDistinctReceivers() public {
        address r0 = _exitReceiver(alice, bob, 1 ether, 0);
        assertEq(desk.nonces(alice), 1);
        address r1 = _exitReceiver(alice, bob, 1 ether, 0);
        assertEq(desk.nonces(alice), 2);

        assertTrue(r0 != r1, "same params, different nonce must differ");
        assertEq(desk.nonces(bob), 0, "nonce is per sender");
    }

    function test_DifferentSendersGetDifferentReceivers() public {
        vm.deal(bob, 10 ether);
        address rAlice = _exitReceiver(alice, bob, 1 ether, 0);
        address rBob = _exitReceiver(bob, bob, 1 ether, 0);
        assertTrue(rAlice != rBob);
        assertEq(desk.nonces(bob), 1);
        assertEq(desk.nonces(alice), 1);
    }

    function test_ZeroAmountReverts() public {
        _onElysium();
        vm.prank(alice);
        vm.expectRevert(ExitDesk.ZeroAmount.selector);
        desk.exitHYPE(bob, 0, false);
        _onHyperEVM();
    }

    function test_TipAboveAmountReverts() public {
        _onElysium();
        vm.prank(alice);
        vm.expectRevert(ExitDesk.TipTooHigh.selector);
        desk.exitHYPE{value: 1 ether}(bob, 1 ether + 1, false);
        _onHyperEVM();
    }

    function test_TipEqualToAmountIsAllowed() public {
        address receiver = _exitReceiver(alice, bob, 1 ether, 1 ether);
        assertEq(arbSys.lastDestination(), receiver);
    }

    function test_ZeroRecipientReverts() public {
        _onElysium();
        vm.prank(alice);
        vm.expectRevert(ExitDesk.ZeroRecipient.selector);
        desk.exitHYPE{value: 1 ether}(address(0), 0, false);
        _onHyperEVM();
    }

    function testFuzz_PreviewParityAndArbSysDestination(
        address sender,
        address recipient,
        uint96 amount,
        uint96 tip,
        bool toCore
    ) public {
        vm.assume(
            uint160(sender) > 0xffff && sender != ARB_SYS && sender != address(desk) && sender != address(factory)
        );
        vm.assume(recipient != address(0));
        amount = uint96(bound(amount, 1, 1_000 ether));
        tip = uint96(bound(tip, 0, amount));
        vm.deal(sender, amount);

        _onElysium();
        (ExitParams memory p, address preview) = desk.previewReceiver(sender, recipient, amount, tip, toCore);
        assertEq(factory.receiverFor(p), preview, "factory parity");

        vm.prank(sender);
        address receiver = desk.exitHYPE{value: amount}(recipient, tip, toCore);

        assertEq(receiver, preview, "exit == preview");
        assertEq(arbSys.lastDestination(), preview, "ArbSys destination");
        assertEq(arbSys.lastValue(), amount, "ArbSys value");
        assertEq(p.sender, sender);
        assertEq(p.recipient, recipient);
        assertEq(p.claimTip, tip);
        assertEq(p.toCore, toCore);
        assertEq(p.chainId, ELYSIUM_CHAIN_ID);
        assertEq(p.desk, address(desk));
        assertEq(p.nonce, 0);
        _onHyperEVM();
    }

    function _previewOnElysium(address sender, address recipient, uint256 amount, uint256 tip)
        internal
        returns (ExitParams memory p, address receiver)
    {
        _onElysium();
        (p, receiver) = desk.previewReceiver(sender, recipient, amount, tip, false);
        _onHyperEVM();
    }

    function _exitReceiver(address sender, address recipient, uint256 amount, uint256 tip)
        internal
        returns (address receiver)
    {
        _onElysium();
        vm.prank(sender);
        receiver = desk.exitHYPE{value: amount}(recipient, tip, false);
        _onHyperEVM();
    }
}
