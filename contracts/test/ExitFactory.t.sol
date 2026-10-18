// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

import {FerryFixture} from "./FerryFixture.sol";
import {ExitParams, ExitParamsLib} from "../src/libraries/ExitParams.sol";
import {ExitFactory} from "../src/hyperevm/ExitFactory.sol";
import {Receiver} from "../src/hyperevm/Receiver.sol";
import {RejectingRecipient} from "./mocks/RejectingRecipient.sol";

contract ExitFactoryTest is FerryFixture {
    uint256 internal constant AMOUNT = 10 ether;
    uint256 internal constant TIP = 0.1 ether;

    // ---------------------------------------------------------------- happy path

    function test_HappyPath() public {
        (ExitParams memory p, address receiver) = _exit(alice, bob, AMOUNT, TIP);
        bytes32 salt = ExitParamsLib.salt(p);
        bytes memory data = _claimCalldata(receiver, AMOUNT, 1);
        vm.deal(address(outbox), AMOUNT);

        vm.expectEmit(true, true, true, true, address(factory));
        emit ExitFactory.Claimed(salt, receiver, bob, AMOUNT - TIP, TIP, keeper);

        vm.prank(keeper);
        uint256 paid = factory.claimAndSweep(data, p);

        assertEq(paid, AMOUNT - TIP);
        assertEq(bob.balance, AMOUNT - TIP, "recipient");
        assertEq(keeper.balance, TIP, "keeper tip");
        assertEq(receiver.balance, 0, "receiver drained");
        assertEq(address(outbox).balance, 0, "outbox paid out");
        assertTrue(receiver.code.length > 0, "receiver deployed");
        assertTrue(factory.isSwept(salt));
        assertTrue(outbox.spent(1));
    }

    function test_HappyPathWithZeroTipSkipsTipTransfer() public {
        (ExitParams memory p, address receiver) = _exit(alice, bob, AMOUNT, 0);
        vm.deal(address(outbox), AMOUNT);
        vm.prank(keeper);
        uint256 paid = factory.claimAndSweep(_claimCalldata(receiver, AMOUNT, 1), p);
        assertEq(paid, AMOUNT);
        assertEq(bob.balance, AMOUNT);
        assertEq(keeper.balance, 0);
    }

    function test_HappyPathWithTipEqualToAmountPaysOnlyKeeper() public {
        (ExitParams memory p, address receiver) = _exit(alice, bob, AMOUNT, AMOUNT);
        vm.deal(address(outbox), AMOUNT);
        vm.prank(keeper);
        uint256 paid = factory.claimAndSweep(_claimCalldata(receiver, AMOUNT, 1), p);
        assertEq(paid, 0);
        assertEq(bob.balance, 0);
        assertEq(keeper.balance, AMOUNT);
    }

    function test_ReceiverAddressIsDeterministicAcrossClaimAndPreview() public {
        (ExitParams memory p, address receiver) = _exit(alice, bob, AMOUNT, TIP);
        assertEq(factory.receiverFor(p), receiver);
        vm.deal(address(outbox), AMOUNT);
        vm.prank(keeper);
        factory.claimAndSweep(_claimCalldata(receiver, AMOUNT, 1), p);
        assertEq(factory.receiverFor(p), receiver, "unchanged after deploy");
        assertEq(address(Receiver(payable(receiver)).factory()), address(factory));
    }

    // ---------------------------------------------------------------- no redirection

    function test_CalldataPayingOtherAddressReverts() public {
        (ExitParams memory p,) = _exit(alice, bob, AMOUNT, TIP);
        vm.deal(address(outbox), AMOUNT);
        vm.prank(keeper);
        vm.expectRevert(ExitFactory.WrongReceiver.selector);
        factory.claimAndSweep(_claimCalldata(keeper, AMOUNT, 1), p);
        assertEq(address(outbox).balance, AMOUNT, "outbox untouched");
    }

    function test_TamperedRecipientProducesDifferentReceiverAndReverts() public {
        (ExitParams memory p, address receiver) = _exit(alice, bob, AMOUNT, TIP);
        ExitParams memory tampered = p;
        tampered.recipient = keeper;
        assertTrue(factory.receiverFor(tampered) != receiver, "different params, different receiver");
        vm.deal(address(outbox), AMOUNT);
        vm.prank(keeper);
        vm.expectRevert(ExitFactory.WrongReceiver.selector);
        factory.claimAndSweep(_claimCalldata(receiver, AMOUNT, 1), tampered);
    }

    function test_TamperedAmountOrTipReverts() public {
        (ExitParams memory p, address receiver) = _exit(alice, bob, AMOUNT, TIP);
        vm.deal(address(outbox), AMOUNT);
        bytes memory data = _claimCalldata(receiver, AMOUNT, 1);

        ExitParams memory q = p;
        q.amount = AMOUNT - 1;
        vm.prank(keeper);
        vm.expectRevert(ExitFactory.WrongReceiver.selector);
        factory.claimAndSweep(data, q);

        q = p;
        q.claimTip = 0;
        vm.prank(keeper);
        vm.expectRevert(ExitFactory.WrongReceiver.selector);
        factory.claimAndSweep(data, q);
    }

    function testFuzz_RecipientAlwaysReceivesRegardlessOfCaller(address caller, uint96 amount, uint96 tip) public {
        vm.assume(uint160(caller) > 0xffff && caller != address(factory) && caller != address(outbox));
        vm.assume(caller != ARB_SYS);
        amount = uint96(bound(amount, 1, 1_000 ether));
        tip = uint96(bound(tip, 0, amount));

        (ExitParams memory p, address receiver) = _exit(alice, bob, amount, tip);
        vm.deal(address(outbox), amount);
        uint256 bobBefore = bob.balance;
        uint256 callerBefore = caller.balance;

        vm.prank(caller);
        factory.claimAndSweep(_claimCalldata(receiver, amount, 1), p);

        uint256 bobGain = bob.balance - bobBefore;
        assertEq(bobGain, amount - tip + (caller == bob ? tip : 0));
        if (caller != bob) assertEq(caller.balance - callerBefore, tip);
    }

    function test_CallerCannotRedirectToSelf() public {
        (ExitParams memory p, address receiver) = _exit(alice, bob, AMOUNT, TIP);
        vm.deal(address(outbox), AMOUNT);
        p.recipient = keeper;
        vm.prank(keeper);
        vm.expectRevert(ExitFactory.WrongReceiver.selector);
        factory.claimAndSweep(_claimCalldata(receiver, AMOUNT, 1), p);
        assertEq(keeper.balance, 0);
    }

    // ---------------------------------------------------------------- input validation

    function test_BadSelectorReverts() public {
        (ExitParams memory p, address receiver) = _exit(alice, bob, AMOUNT, TIP);
        bytes memory data = abi.encodeWithSignature("transfer(address,uint256)", receiver, AMOUNT);
        vm.prank(keeper);
        vm.expectRevert(ExitFactory.BadSelector.selector);
        factory.claimAndSweep(data, p);
    }

    function test_ShortCalldataReverts() public {
        (ExitParams memory p,) = _exit(alice, bob, AMOUNT, TIP);
        vm.startPrank(keeper);
        vm.expectRevert(ExitFactory.BadSelector.selector);
        factory.claimAndSweep(new bytes(0), p);
        vm.expectRevert(ExitFactory.BadSelector.selector);
        factory.claimAndSweep(hex"08635a", p);
        vm.stopPrank();
    }

    function test_TipAboveAmountReverts() public {
        (ExitParams memory p, address receiver) = _exit(alice, bob, AMOUNT, TIP);
        p.claimTip = AMOUNT + 1;
        vm.prank(keeper);
        vm.expectRevert(ExitFactory.TipTooHigh.selector);
        factory.claimAndSweep(_claimCalldata(receiver, AMOUNT, 1), p);
    }

    // ---------------------------------------------------------------- idempotency and claim failures

    function test_SecondClaimRevertsAlreadySwept() public {
        (ExitParams memory p, address receiver) = _exit(alice, bob, AMOUNT, TIP);
        vm.deal(address(outbox), AMOUNT);
        bytes memory data = _claimCalldata(receiver, AMOUNT, 1);
        vm.prank(keeper);
        factory.claimAndSweep(data, p);

        vm.prank(keeper);
        vm.expectRevert(ExitFactory.AlreadySwept.selector);
        factory.claimAndSweep(data, p);
    }

    function test_ClaimSucceedsWhenOutboxAlreadyExecutedElsewhere() public {
        (ExitParams memory p, address receiver) = _exit(alice, bob, AMOUNT, TIP);
        vm.deal(address(outbox), AMOUNT);
        bytes memory data = _claimCalldata(receiver, AMOUNT, 1);

        // Someone executes the Outbox message directly. The receiver is now funded.
        (bool ok,) = address(outbox).call(data);
        assertTrue(ok);
        assertEq(receiver.balance, AMOUNT);

        // The Outbox call in claimAndSweep reverts with ALREADY_SPENT, but the balance covers it.
        vm.prank(keeper);
        uint256 paid = factory.claimAndSweep(data, p);
        assertEq(paid, AMOUNT - TIP);
        assertEq(bob.balance, AMOUNT - TIP);
        assertEq(keeper.balance, TIP);
        assertEq(receiver.balance, 0);
        assertTrue(factory.isSwept(ExitParamsLib.salt(p)));
    }

    function test_ClaimRevertsNotClaimableWhenNeitherClaimedNorFunded() public {
        (ExitParams memory p, address receiver) = _exit(alice, bob, AMOUNT, TIP);
        // Outbox has no balance, so the claim call reverts; the receiver is empty.
        vm.prank(keeper);
        vm.expectRevert(ExitFactory.NotClaimable.selector);
        factory.claimAndSweep(_claimCalldata(receiver, AMOUNT, 1), p);
        assertFalse(factory.isSwept(ExitParamsLib.salt(p)));
        assertEq(receiver.code.length, 0, "no deploy on failed claim");
    }

    function test_ClaimRevertsNotClaimableWhenMessageSpentAndNoFunds() public {
        // The Outbox message was already executed (index 7) but the receiver holds nothing.
        // The Outbox call reverts ALREADY_SPENT and the balance check fails.
        (ExitParams memory p, address receiver) = _exit(alice, bob, AMOUNT, TIP);
        vm.deal(address(outbox), AMOUNT);
        (bool ok,) = address(outbox).call(_claimCalldata(makeAddr("elsewhere"), AMOUNT, 7));
        assertTrue(ok, "pre-spend index 7 for an unrelated payee");

        vm.prank(keeper);
        vm.expectRevert(ExitFactory.NotClaimable.selector);
        factory.claimAndSweep(_claimCalldata(receiver, AMOUNT, 7), p);
        assertFalse(factory.isSwept(ExitParamsLib.salt(p)));
    }

    function test_FailedClaimDoesNotDeployOrMarkSwept() public {
        (ExitParams memory p, address receiver) = _exit(alice, bob, AMOUNT, TIP);
        vm.deal(address(outbox), AMOUNT - 1 ether); // underfunded outbox
        vm.prank(keeper);
        vm.expectRevert(ExitFactory.NotClaimable.selector);
        factory.claimAndSweep(_claimCalldata(receiver, AMOUNT, 1), p);
        assertEq(receiver.balance, 0);
        assertFalse(factory.isSwept(ExitParamsLib.salt(p)));
    }

    // ---------------------------------------------------------------- pre-funding and sweeping

    function test_StrayFundsBeforeClaimStayForSweep() public {
        (ExitParams memory p, address receiver) = _exit(alice, bob, AMOUNT, TIP);
        _fund(receiver, 1 ether); // undeployed receiver accepts ETH
        vm.deal(address(outbox), AMOUNT);

        vm.prank(keeper);
        factory.claimAndSweep(_claimCalldata(receiver, AMOUNT, 1), p);
        assertEq(bob.balance, AMOUNT - TIP);
        assertEq(receiver.balance, 1 ether, "stray kept until sweep");

        factory.sweep(p);
        assertEq(bob.balance, AMOUNT - TIP + 1 ether);
        assertEq(receiver.balance, 0);
    }

    function test_SweepSendsLateStrayFundsToRecipient() public {
        (ExitParams memory p, address receiver) = _exit(alice, bob, AMOUNT, TIP);
        vm.deal(address(outbox), AMOUNT);
        vm.prank(keeper);
        factory.claimAndSweep(_claimCalldata(receiver, AMOUNT, 1), p);

        _fund(receiver, 2 ether);
        vm.prank(alice); // anyone may sweep
        factory.sweep(p);
        assertEq(bob.balance, AMOUNT - TIP + 2 ether);
        assertEq(receiver.balance, 0);
    }

    function test_SweepBeforeClaimReverts() public {
        (ExitParams memory p, address receiver) = _exit(alice, bob, AMOUNT, TIP);
        _fund(receiver, 1 ether);
        vm.expectRevert(ExitFactory.NotSwept.selector);
        factory.sweep(p);
    }

    function test_SweepWithNothingLeftReverts() public {
        (ExitParams memory p, address receiver) = _exit(alice, bob, AMOUNT, TIP);
        vm.deal(address(outbox), AMOUNT);
        vm.prank(keeper);
        factory.claimAndSweep(_claimCalldata(receiver, AMOUNT, 1), p);
        vm.expectRevert(ExitFactory.NothingToSweep.selector);
        factory.sweep(p);
    }

    function test_SweepRevertsIfRecipientRejectsEth() public {
        RejectingRecipient bad = new RejectingRecipient();
        (ExitParams memory p, address receiver) = _exit(alice, address(bad), AMOUNT, TIP);
        vm.deal(address(outbox), AMOUNT);
        vm.prank(keeper);
        vm.expectRevert(Receiver.TransferFailed.selector);
        factory.claimAndSweep(_claimCalldata(receiver, AMOUNT, 1), p);
        assertEq(receiver.balance, 0, "unchanged: nothing paid");
        assertFalse(factory.isSwept(ExitParamsLib.salt(p)));
    }

    // ---------------------------------------------------------------- reentrancy

    function test_ReentrancyLockBlocksNestedClaim() public {
        // A recipient that tries to call back into the factory on receipt must be blocked.
        ReentrantRecipient evil = new ReentrantRecipient(factory);
        (ExitParams memory p, address receiver) = _exit(alice, address(evil), AMOUNT, 0);
        evil.arm(p, _claimCalldata(receiver, AMOUNT, 2));
        vm.deal(address(outbox), AMOUNT);

        vm.prank(keeper);
        factory.claimAndSweep(_claimCalldata(receiver, AMOUNT, 1), p);
        assertTrue(evil.attempted());
        assertFalse(evil.reentrySucceeded());
        assertEq(address(evil).balance, AMOUNT);
    }

}

/// @dev Tries to re-enter claimAndSweep when it receives HYPE.
contract ReentrantRecipient {
    ExitFactory internal immutable target;
    ExitParams internal armedParams;
    bytes internal armedData;
    bool public armed;
    bool public attempted;
    bool public reentrySucceeded;

    constructor(ExitFactory target_) {
        target = target_;
    }

    function arm(ExitParams memory p, bytes memory data) external {
        armedParams = p;
        armedData = data;
        armed = true;
    }

    receive() external payable {
        if (!armed) return;
        armed = false;
        attempted = true;
        try target.claimAndSweep(armedData, armedParams) returns (uint256) {
            reentrySucceeded = true;
        } catch {}
    }
}
