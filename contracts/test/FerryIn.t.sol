// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

import {PoolKey} from "../src/libraries/InfinityTypes.sol";
import {InfinitySwapAdapter} from "../src/elysium/InfinitySwapAdapter.sol";
import {FerryIn} from "../src/elysium/FerryIn.sol";
import {FeeJar} from "../src/elysium/FeeJar.sol";
import {IERC20} from "../src/interfaces/IERC20.sol";
import {MockCLPoolManager} from "./mocks/MockCLPoolManager.sol";
import {MockERC20} from "./mocks/MockERC20.sol";
import {RejectingRecipient} from "./mocks/RejectingRecipient.sol";
import {ToggleRecipient} from "./mocks/ToggleRecipient.sol";
import {InfinityFixture} from "./InfinityFixture.sol";

contract FerryInTest is InfinityFixture {
    // 10 HYPE: fee 0.03, swap 9.97 -> 29.91 usdcA -> 59.82 tokenA.
    uint256 internal constant HYPE_IN = 10 ether;
    uint256 internal constant FEE_10 = 0.03 ether;
    uint256 internal constant OUT_TOKEN_A = 59.82 ether;

    function _buy(address who, address token, address recipient, uint256 minOut, uint256 value) internal {
        vm.prank(who);
        ferry.buy{value: value}(token, recipient, minOut);
    }

    // ---------------------------------------------------------------- happy paths (F1, F2)

    function test_BuyHappyPath() public {
        assertEq(ferry.quote(address(tokenA), HYPE_IN), OUT_TOKEN_A);

        vm.expectEmit(true, true, false, true, address(ferry));
        emit FerryIn.Bought(address(tokenA), bob, HYPE_IN, FEE_10, OUT_TOKEN_A);
        _buy(alice, address(tokenA), bob, OUT_TOKEN_A, HYPE_IN);

        assertEq(tokenA.balanceOf(bob), OUT_TOKEN_A);
        assertEq(address(feeJar).balance, FEE_10, "fee to FeeJar");
        assertEq(address(ferry).balance, 0, "ferry holds HYPE");
        assertEq(address(adapter).balance, 0, "adapter holds HYPE");
        assertEq(bob.balance, 0, "recipient got no HYPE back");
    }

    function test_ZeroRecipientDefaultsToSender() public {
        _buy(alice, address(tokenA), address(0), OUT_TOKEN_A, HYPE_IN);
        assertEq(tokenA.balanceOf(alice), 1e24 + OUT_TOKEN_A);
    }

    function test_TokenLaunchedByAnotherAddressWorks() public {
        // F2: carol launched tokenC. Anyone can register it, and the buy works the same way.
        vm.expectEmit(true, true, false, true, address(ferry));
        emit FerryIn.TokenRegistered(address(tokenC), address(usdcA), poolC);
        vm.prank(carol);
        ferry.registerToken(poolC);
        assertTrue(ferry.isRegistered(address(tokenC)));
        assertEq(ferry.poolKeyOf(address(tokenC)).fee, FEE);

        // 9.97 HYPE-equivalent: 29.91 usdcA -> 14.955 tokenC
        _buy(alice, address(tokenC), bob, 14.955 ether, HYPE_IN);
        assertEq(tokenC.balanceOf(bob), 14.955 ether);
        assertEq(address(feeJar).balance, FEE_10);
    }

    function test_TwoQuoteCurrenciesRouteCorrectly() public {
        // tokenA quotes in usdcA (native HYPE route); tokenB quotes in usdcB (WHYPE route).
        assertEq(ferry.quote(address(tokenA), HYPE_IN), OUT_TOKEN_A);
        assertEq(ferry.quote(address(tokenB), HYPE_IN), 9.97 ether * 2 * 4);

        assertTrue(ferry.isQuoteCurrency(address(usdcA)));
        assertTrue(ferry.isQuoteCurrency(address(usdcB)));
        assertFalse(ferry.isQuoteCurrency(address(tokenA)));

        _buy(alice, address(tokenB), bob, 79.76 ether, HYPE_IN);
        assertEq(tokenB.balanceOf(bob), 79.76 ether);
        assertEq(usdcB.balanceOf(address(adapter)), 0);
    }

    // ---------------------------------------------------------------- F3: failed buys refund on Elysium

    function test_F3ImpossibleMinOutRefundsFullValue() public {
        bytes memory reason =
            abi.encodeWithSelector(InfinitySwapAdapter.InsufficientOutput.selector, OUT_TOKEN_A, type(uint256).max);

        vm.expectEmit(true, true, false, true, address(ferry));
        emit FerryIn.Refunded(address(tokenA), bob, HYPE_IN, reason);
        _buy(alice, address(tokenA), bob, type(uint256).max, HYPE_IN);

        assertEq(bob.balance, HYPE_IN, "full value refunded, fee included");
        assertEq(address(feeJar).balance, 0, "no fee on a failed buy");
        assertEq(tokenA.balanceOf(bob), 0);
        assertEq(address(ferry).balance, 0);
    }

    function test_PausedPoolRefunds() public {
        pm.setPaused(poolA, true);
        bytes memory reason = abi.encodeWithSelector(MockCLPoolManager.PoolPaused.selector);

        vm.expectEmit(true, true, false, true, address(ferry));
        emit FerryIn.Refunded(address(tokenA), bob, HYPE_IN, reason);
        _buy(alice, address(tokenA), bob, 0, HYPE_IN);

        assertEq(bob.balance, HYPE_IN);
        assertEq(address(feeJar).balance, 0);
    }

    function test_PausedRouteRefunds() public {
        pm.setPaused(routeNative, true);
        _buy(alice, address(tokenA), bob, 0, HYPE_IN);
        assertEq(bob.balance, HYPE_IN);
    }

    function test_UnregisteredTokenRefunds() public {
        address ghost = makeAddr("ghost");
        bytes memory reason = abi.encodeWithSelector(FerryIn.NotRegistered.selector);

        vm.expectEmit(true, true, false, true, address(ferry));
        emit FerryIn.Refunded(ghost, bob, HYPE_IN, reason);
        _buy(alice, ghost, bob, 0, HYPE_IN);

        assertEq(bob.balance, HYPE_IN);
    }

    function test_RefundToRejectingRecipientFallsBackToWhype() public {
        RejectingRecipient rejecting = new RejectingRecipient();
        pm.setPaused(poolA, true);

        _buy(alice, address(tokenA), address(rejecting), 0, HYPE_IN);

        assertEq(whype.balanceOf(address(rejecting)), HYPE_IN, "refund arrives as WHYPE");
        assertEq(address(ferry).balance, 0);
        assertEq(ferry.owed(address(rejecting)), 0);
    }

    function test_UndeliverableRefundIsOwedAndClaimable() public {
        ToggleRecipient recipient = new ToggleRecipient();
        pm.setPaused(poolA, true);
        // Native refund fails (accept=false) and the WHYPE transfer also fails, so the value is owed.
        vm.mockCallRevert(address(whype), abi.encodeCall(IERC20.transfer, (address(recipient), HYPE_IN)), bytes(""));

        vm.expectEmit(true, false, false, true, address(ferry));
        emit FerryIn.Owed(address(recipient), HYPE_IN);
        _buy(alice, address(tokenA), address(recipient), 0, HYPE_IN);

        assertEq(ferry.owed(address(recipient)), HYPE_IN);
        assertEq(address(ferry).balance, HYPE_IN, "owed value is held as native");
        assertEq(whype.balanceOf(address(ferry)), 0, "WHYPE was unwrapped back");

        vm.expectRevert(FerryIn.TransferFailed.selector);
        recipient.claim(ferry);

        recipient.setAccept(true);
        recipient.claim(ferry);
        assertEq(ferry.owed(address(recipient)), 0);
        assertEq(address(recipient).balance, HYPE_IN);
        assertEq(address(ferry).balance, 0);

        vm.expectRevert(FerryIn.NothingOwed.selector);
        recipient.claim(ferry);
    }

    function test_FeeJarFailureSendsFeeToRecipient() public {
        // FeeJar rejects HYPE here, so the fee goes to the recipient. The buy still succeeds.
        FerryIn ferryNoJar = _deployFerry(address(new RejectingRecipient()));
        ferryNoJar.registerToken(poolA);

        vm.prank(alice);
        ferryNoJar.buy{value: HYPE_IN}(address(tokenA), bob, OUT_TOKEN_A);

        assertEq(tokenA.balanceOf(bob), OUT_TOKEN_A);
        assertEq(bob.balance, FEE_10, "fee redirected to recipient");
        assertEq(address(ferryNoJar).balance, 0);
    }

    function test_ZeroValueReverts() public {
        vm.prank(alice);
        vm.expectRevert(FerryIn.ZeroValue.selector);
        ferry.buy(address(tokenA), bob, 0);
    }

    function test_ClaimOwedWithNothingReverts() public {
        vm.prank(bob);
        vm.expectRevert(FerryIn.NothingOwed.selector);
        ferry.claimOwed();
    }

    function test_QuoteUnregisteredReverts() public {
        vm.expectRevert(FerryIn.NotRegistered.selector);
        ferry.quote(makeAddr("ghost"), HYPE_IN);
    }

    function test_ReceiveRejectsNonWhype() public {
        vm.deal(address(this), 1 ether);
        vm.expectRevert(FerryIn.UnexpectedValue.selector);
        address(ferry).call{value: 1 ether}("");
        assertEq(address(ferry).balance, 0);
    }

    // ---------------------------------------------------------------- registration rules

    function test_RegisterRejectsNonAscendHooks() public {
        PoolKey memory key = _key(address(usdcA), address(tokenC), otherHooks, pm);
        vm.expectRevert(FerryIn.NotAscendPool.selector);
        ferry.registerToken(key);
    }

    function test_RegisterRejectsUnknownPoolManager() public {
        PoolKey memory key = _pool(address(usdcA), address(tokenC), launchpad, roguePm, ONE);
        vm.expectRevert(abi.encodeWithSelector(FerryIn.UnknownPoolManager.selector, address(roguePm)));
        ferry.registerToken(key);
    }

    function test_RegisterRejectsUninitializedPool() public {
        MockERC20 fresh = new MockERC20("Fresh", "FRS", 18);
        PoolKey memory key = _key(address(usdcA), address(fresh), launchpad, pm);
        vm.expectRevert(FerryIn.PoolNotInitialized.selector);
        ferry.registerToken(key);
    }

    function test_RegisterRejectsDuplicate() public {
        vm.expectRevert(FerryIn.AlreadyRegistered.selector);
        ferry.registerToken(poolA);
    }

    function test_RegisterRejectsPoolWithoutSupportedQuote() public {
        // tokenA/tokenC: neither side is a quote currency.
        PoolKey memory noQuote = _key(address(tokenA), address(tokenC), launchpad, pm);
        vm.expectRevert(FerryIn.UnsupportedQuote.selector);
        ferry.registerToken(noQuote);

        // usdcA/usdcB: both sides are quote currencies.
        PoolKey memory bothQuotes = _key(address(usdcA), address(usdcB), launchpad, pm);
        vm.expectRevert(FerryIn.UnsupportedQuote.selector);
        ferry.registerToken(bothQuotes);
    }

    function test_RegisterRejectsHypeAsToken() public {
        PoolKey memory hypePool = _key(address(whype), address(usdcA), launchpad, pm);
        vm.expectRevert(FerryIn.UnsupportedQuote.selector);
        ferry.registerToken(hypePool);
    }

    function test_RegisterRejectsNativeAsToken() public {
        PoolKey memory nativePool = _key(address(0), address(usdcA), launchpad, pm);
        vm.expectRevert(FerryIn.UnsupportedQuote.selector);
        ferry.registerToken(nativePool);
    }

    // ---------------------------------------------------------------- constructor rules

    function test_ConstructorRejectsRouteWithoutHype() public {
        PoolKey[] memory routes = new PoolKey[](1);
        routes[0] = _pool(address(usdcA), address(usdcB), otherHooks, pm, ONE);
        vm.expectRevert(FerryIn.BadRoute.selector);
        new FerryIn(address(adapter), address(feeJar), launchpad, address(whype), routes);
    }

    function test_ConstructorRejectsRouteWithHypeOnBothSides() public {
        PoolKey[] memory routes = new PoolKey[](1);
        routes[0] = _pool(address(0), address(whype), otherHooks, pm, ONE);
        vm.expectRevert(FerryIn.BadRoute.selector);
        new FerryIn(address(adapter), address(feeJar), launchpad, address(whype), routes);
    }

    function test_ConstructorRejectsDuplicateQuote() public {
        PoolKey[] memory routes = new PoolKey[](2);
        routes[0] = routeNative;
        routes[1] = _pool(address(whype), address(usdcA), otherHooks, pm, 2 * ONE);
        vm.expectRevert(FerryIn.DuplicateRoute.selector);
        new FerryIn(address(adapter), address(feeJar), launchpad, address(whype), routes);
    }

    function test_ConstructorRejectsMixedPoolManagers() public {
        PoolKey[] memory routes = new PoolKey[](2);
        routes[0] = routeNative;
        routes[1] = _pool(address(whype), address(usdcB), otherHooks, roguePm, 2 * ONE);
        vm.expectRevert(abi.encodeWithSelector(FerryIn.UnknownPoolManager.selector, address(roguePm)));
        new FerryIn(address(adapter), address(feeJar), launchpad, address(whype), routes);
    }

    function test_ConstructorRejectsUninitializedRoute() public {
        PoolKey[] memory routes = new PoolKey[](1);
        routes[0] = _key(address(0), address(tokenC), otherHooks, pm);
        vm.expectRevert(FerryIn.PoolNotInitialized.selector);
        new FerryIn(address(adapter), address(feeJar), launchpad, address(whype), routes);
    }

    function test_ConstructorRejectsEmptyRoutes() public {
        PoolKey[] memory routes = new PoolKey[](0);
        vm.expectRevert(FerryIn.NoRoutes.selector);
        new FerryIn(address(adapter), address(feeJar), launchpad, address(whype), routes);
    }

    function test_ConstructorRejectsZeroAddresses() public {
        PoolKey[] memory routes = new PoolKey[](1);
        routes[0] = routeNative;
        vm.expectRevert(FerryIn.ZeroAddress.selector);
        new FerryIn(address(adapter), address(0), launchpad, address(whype), routes);
    }

    // ---------------------------------------------------------------- fuzz

    /// @dev Every rate in the fixture is an exact multiple, so output is (hypeIn - fee) * 3 * 2.
    function testFuzz_BuyPaysFeeAndQuote(uint256 hypeIn) public {
        hypeIn = bound(hypeIn, 1e15, 1000 ether);
        vm.deal(alice, hypeIn);

        uint256 fee = hypeIn * 30 / 10_000;
        uint256 expected = (hypeIn - fee) * 6;
        assertEq(ferry.quote(address(tokenA), hypeIn), expected, "quote");

        _buy(alice, address(tokenA), bob, expected, hypeIn);

        assertEq(tokenA.balanceOf(bob), expected, "recipient");
        assertEq(address(feeJar).balance, fee, "fee");
        assertEq(address(ferry).balance, 0, "ferry");
        assertEq(address(adapter).balance, 0, "adapter");
        assertEq(bob.balance, 0, "no HYPE refunded on success");
    }

    function testFuzz_MinOutAboveQuoteRefundsFullValue(uint256 hypeIn, uint256 surplus) public {
        hypeIn = bound(hypeIn, 1e15, 1000 ether);
        surplus = bound(surplus, 1, 1e30);
        vm.deal(alice, hypeIn);

        uint256 expected = (hypeIn - hypeIn * 30 / 10_000) * 6;
        _buy(alice, address(tokenA), bob, expected + surplus, hypeIn);

        assertEq(bob.balance, hypeIn, "refund");
        assertEq(address(feeJar).balance, 0, "fee");
        assertEq(tokenA.balanceOf(bob), 0, "tokens");
    }
}
