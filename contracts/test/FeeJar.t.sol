// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

import {Test} from "forge-std/Test.sol";
import {FeeJar} from "../src/elysium/FeeJar.sol";
import {RejectingRecipient} from "./mocks/RejectingRecipient.sol";

contract FeeJarTest is Test {
    address internal beneficiary = makeAddr("beneficiary");
    address internal anyone = makeAddr("anyone");
    FeeJar internal jar;

    function setUp() public {
        jar = new FeeJar(beneficiary);
    }

    function test_BeneficiaryIsImmutable() public view {
        assertEq(jar.beneficiary(), beneficiary);
    }

    function test_ZeroBeneficiaryReverts() public {
        vm.expectRevert(FeeJar.ZeroAddress.selector);
        new FeeJar(address(0));
    }

    function test_ReceiveAccumulatesFees() public {
        vm.deal(anyone, 3 ether);
        vm.prank(anyone);
        (bool ok,) = address(jar).call{value: 1 ether}("");
        assertTrue(ok);
        vm.prank(anyone);
        (ok,) = address(jar).call{value: 2 ether}("");
        assertTrue(ok);
        assertEq(address(jar).balance, 3 ether);
    }

    function test_ReleaseSendsFullBalanceToBeneficiary() public {
        vm.deal(address(jar), 2.5 ether);
        vm.expectEmit(true, true, true, true, address(jar));
        emit FeeJar.Released(2.5 ether);
        vm.prank(anyone); // permissionless
        jar.release();
        assertEq(beneficiary.balance, 2.5 ether);
        assertEq(address(jar).balance, 0);
    }

    function test_ReleaseWithEmptyBalanceIsNoop() public {
        vm.expectEmit(true, true, true, true, address(jar));
        emit FeeJar.Released(0);
        jar.release();
        assertEq(beneficiary.balance, 0);
    }

    function test_ReleaseRevertsIfBeneficiaryRejects() public {
        RejectingRecipient bad = new RejectingRecipient();
        FeeJar badJar = new FeeJar(address(bad));
        vm.deal(address(badJar), 1 ether);
        vm.expectRevert(FeeJar.TransferFailed.selector);
        badJar.release();
        assertEq(address(badJar).balance, 1 ether);
    }
}
