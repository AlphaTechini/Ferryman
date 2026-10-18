// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

import {Test} from "forge-std/Test.sol";
import {ExitParams, ExitParamsLib} from "../src/libraries/ExitParams.sol";
import {Receiver} from "../src/hyperevm/Receiver.sol";
import {RejectingRecipient} from "./mocks/RejectingRecipient.sol";
import {FerryFixture} from "./FerryFixture.sol";

/// @dev Receiver built directly with this test contract as its factory, so pull() can be exercised.
contract ReceiverTest is Test {
    address internal bob = makeAddr("bob");
    address internal alice = makeAddr("alice");
    Receiver internal r;

    function setUp() public {
        r = new Receiver(address(this));
    }

    function test_ReceiveAcceptsEth() public {
        (bool ok,) = address(r).call{value: 1 ether}("");
        assertTrue(ok);
        assertEq(address(r).balance, 1 ether);
    }

    function test_FactoryIsImmutableAndSet() public view {
        assertEq(r.factory(), address(this));
    }

    function test_PullOnlyCallableByFactory() public {
        vm.deal(address(r), 1 ether);
        vm.prank(alice);
        vm.expectRevert(Receiver.NotFactory.selector);
        r.pull(bob, 1 ether);
        assertEq(address(r).balance, 1 ether);
    }

    function test_FactoryCanPull() public {
        vm.deal(address(r), 1 ether);
        r.pull(bob, 0.4 ether);
        assertEq(bob.balance, 0.4 ether);
        assertEq(address(r).balance, 0.6 ether);
    }

    function test_PullRevertsTransferFailedWhenRecipientRejects() public {
        RejectingRecipient bad = new RejectingRecipient();
        vm.deal(address(r), 1 ether);
        vm.expectRevert(Receiver.TransferFailed.selector);
        r.pull(address(bad), 1 ether);
        assertEq(address(r).balance, 1 ether);
    }
}

/// @dev Receiver deployed by ExitFactory at its CREATE2 address: accepts funds before and after deploy.
contract ReceiverAtCreate2AddressTest is FerryFixture {
    function test_ReceiveBeforeDeployIsKept() public {
        ExitParams memory p;
        address receiver;
        (p, receiver) = desk.previewReceiver(alice, bob, 2 ether, 0, false);
        assertEq(receiver.code.length, 0, "not deployed yet");

        _fund(receiver, 1 ether);
        assertEq(receiver.balance, 1 ether);

        // Claim deploys the receiver at the same address; the early balance is preserved.
        _exit(alice, bob, 2 ether, 0);
        vm.deal(address(outbox), 2 ether);
        vm.prank(keeper);
        factory.claimAndSweep(_claimCalldata(receiver, 2 ether, 1), p);
        assertTrue(receiver.code.length > 0);
        assertEq(receiver.balance, 1 ether, "pre-deploy funds untouched by claim");
    }

    function test_ReceiveAfterDeployAccepted() public {
        (ExitParams memory p, address receiver) = _exit(alice, bob, 1 ether, 0);
        vm.deal(address(outbox), 1 ether);
        vm.prank(keeper);
        factory.claimAndSweep(_claimCalldata(receiver, 1 ether, 1), p);

        _fund(receiver, 0.5 ether);
        assertEq(receiver.balance, 0.5 ether);
        factory.sweep(p);
        assertEq(bob.balance, 1.5 ether);
    }

    function test_DeployedReceiverPointsAtFactory() public {
        (ExitParams memory p, address receiver) = _exit(alice, bob, 1 ether, 0);
        vm.deal(address(outbox), 1 ether);
        vm.prank(keeper);
        factory.claimAndSweep(_claimCalldata(receiver, 1 ether, 1), p);
        assertEq(Receiver(payable(receiver)).factory(), address(factory));

        // Only the factory can pull from the deployed receiver.
        vm.prank(alice);
        vm.expectRevert(Receiver.NotFactory.selector);
        Receiver(payable(receiver)).pull(alice, 0);
    }
}
