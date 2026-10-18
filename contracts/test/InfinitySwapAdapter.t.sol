// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

import {PoolKey} from "../src/libraries/InfinityTypes.sol";
import {InfinitySwapAdapter} from "../src/elysium/InfinitySwapAdapter.sol";
import {MockCLPoolManager} from "./mocks/MockCLPoolManager.sol";
import {InfinityFixture} from "./InfinityFixture.sol";

contract InfinitySwapAdapterTest is InfinityFixture {
    function _assertAdapterEmpty() internal view {
        assertEq(address(adapter).balance, 0, "adapter holds native");
        assertEq(usdcA.balanceOf(address(adapter)), 0, "adapter holds usdcA");
        assertEq(usdcB.balanceOf(address(adapter)), 0, "adapter holds usdcB");
        assertEq(tokenA.balanceOf(address(adapter)), 0, "adapter holds tokenA");
        assertEq(tokenB.balanceOf(address(adapter)), 0, "adapter holds tokenB");
        assertEq(whype.balanceOf(address(adapter)), 0, "adapter holds WHYPE");
    }

    function test_SingleHopNativeInput() public {
        vm.prank(alice);
        uint256 out = adapter.swapExactIn{value: 1 ether}(_one(routeNative), address(0), 1 ether, 0, bob);
        assertEq(out, 3 ether);
        assertEq(usdcA.balanceOf(bob), 3 ether);
        _assertAdapterEmpty();
    }

    function test_SingleHopErc20Input() public {
        vm.startPrank(alice);
        usdcA.approve(address(adapter), 10 ether);
        uint256 out = adapter.swapExactIn(_one(poolA), address(usdcA), 10 ether, 0, bob);
        vm.stopPrank();
        assertEq(out, 20 ether);
        assertEq(tokenA.balanceOf(bob), 20 ether);
        assertEq(usdcA.balanceOf(alice), 1e24 - 10 ether);
        _assertAdapterEmpty();
    }

    function test_TwoHopNativeThroughWhypeRoute() public {
        // HYPE is wrapped to WHYPE, then 1 WHYPE -> 2 usdcB -> 8 tokenB.
        vm.prank(alice);
        uint256 out = adapter.swapExactIn{value: 1 ether}(_two(routeWhype, poolB), address(0), 1 ether, 0, bob);
        assertEq(out, 8 ether);
        assertEq(tokenB.balanceOf(bob), 8 ether);
        _assertAdapterEmpty();
    }

    function test_TwoHopNativeThroughNativeRoute() public {
        // 1 HYPE -> 3 usdcA -> 6 tokenA. Intermediate usdcA nets to zero inside the lock.
        vm.prank(alice);
        uint256 out = adapter.swapExactIn{value: 1 ether}(_two(routeNative, poolA), address(0), 1 ether, 0, bob);
        assertEq(out, 6 ether);
        assertEq(tokenA.balanceOf(bob), 6 ether);
        // The intermediate usdcA never leaves the vault, so the vault's usdcA balance is unchanged.
        assertEq(usdcA.balanceOf(address(vault)), 1e30);
        _assertAdapterEmpty();
    }

    function test_WhypeErc20InputThroughWhypeRoute() public {
        vm.startPrank(alice);
        whype.deposit{value: 2 ether}();
        whype.approve(address(adapter), 2 ether);
        uint256 out = adapter.swapExactIn(_one(routeWhype), address(whype), 2 ether, 0, bob);
        vm.stopPrank();
        assertEq(out, 4 ether);
        assertEq(usdcB.balanceOf(bob), 4 ether);
        _assertAdapterEmpty();
    }

    function test_MinOutExactlyMetSucceeds() public {
        vm.prank(alice);
        uint256 out = adapter.swapExactIn{value: 1 ether}(_one(routeNative), address(0), 1 ether, 3 ether, bob);
        assertEq(out, 3 ether);
    }

    function test_MinOutEnforced() public {
        vm.prank(alice);
        vm.expectRevert(abi.encodeWithSelector(InfinitySwapAdapter.InsufficientOutput.selector, 3 ether, 3 ether + 1));
        adapter.swapExactIn{value: 1 ether}(_one(routeNative), address(0), 1 ether, 3 ether + 1, bob);
        assertEq(usdcA.balanceOf(bob), 0);
    }

    function test_QuoteMatchesSwapAndMovesNothing() public {
        uint256 quoted = adapter.quoteExactIn(_two(routeWhype, poolB), address(0), 1 ether);
        assertEq(quoted, 8 ether);
        // The quote reverts inside the lock, so no balances or vault deltas change.
        assertEq(tokenB.balanceOf(bob), 0);
        assertEq(vault.currencyDelta(address(adapter), address(tokenB)), 0);
        _assertAdapterEmpty();

        vm.prank(alice);
        uint256 out = adapter.swapExactIn{value: 1 ether}(_two(routeWhype, poolB), address(0), 1 ether, 0, bob);
        assertEq(out, quoted);
    }

    function test_NativeValueMismatchReverts() public {
        vm.prank(alice);
        vm.expectRevert(InfinitySwapAdapter.NativeValueMismatch.selector);
        adapter.swapExactIn{value: 1 ether}(_one(routeNative), address(0), 2 ether, 0, bob);
    }

    function test_ValueWithErc20InputReverts() public {
        vm.startPrank(alice);
        usdcA.approve(address(adapter), 1 ether);
        vm.expectRevert(InfinitySwapAdapter.UnexpectedValue.selector);
        adapter.swapExactIn{value: 1 wei}(_one(poolA), address(usdcA), 1 ether, 0, bob);
        vm.stopPrank();
    }

    function test_InputGuards() public {
        vm.startPrank(alice);
        vm.expectRevert(InfinitySwapAdapter.ZeroAmount.selector);
        adapter.swapExactIn(_one(routeNative), address(0), 0, 0, bob);

        vm.expectRevert(InfinitySwapAdapter.ZeroRecipient.selector);
        adapter.swapExactIn{value: 1 ether}(_one(routeNative), address(0), 1 ether, 0, address(0));

        vm.expectRevert(InfinitySwapAdapter.BadPath.selector);
        adapter.swapExactIn{value: 1 ether}(new PoolKey[](0), address(0), 1 ether, 0, bob);

        vm.expectRevert(InfinitySwapAdapter.AmountTooLarge.selector);
        adapter.swapExactIn(_one(routeNative), address(0), 2 ** 128, 0, bob);
        vm.stopPrank();
    }

    function test_BadPathWhenInputIsNotInPool() public {
        vm.startPrank(alice);
        usdcB.approve(address(adapter), 1 ether);
        vm.expectRevert(InfinitySwapAdapter.BadPath.selector);
        adapter.swapExactIn(_one(poolA), address(usdcB), 1 ether, 0, bob);
        vm.stopPrank();
    }

    function test_BadPathWhenHopsDoNotChain() public {
        // Output of routeNative is usdcA, but poolB only takes usdcB.
        vm.prank(alice);
        vm.expectRevert(InfinitySwapAdapter.BadPath.selector);
        adapter.swapExactIn{value: 1 ether}(_two(routeNative, poolB), address(0), 1 ether, 0, bob);
    }

    function test_PausedPoolReverts() public {
        pm.setPaused(poolA, true);
        vm.startPrank(alice);
        usdcA.approve(address(adapter), 1 ether);
        vm.expectRevert(MockCLPoolManager.PoolPaused.selector);
        adapter.swapExactIn(_one(poolA), address(usdcA), 1 ether, 0, bob);
        vm.stopPrank();
    }

    function test_UninitializedPoolReverts() public {
        PoolKey memory ghost = _key(address(usdcA), address(tokenA), otherHooks, pm);
        vm.startPrank(alice);
        usdcA.approve(address(adapter), 1 ether);
        vm.expectRevert(MockCLPoolManager.PoolNotInitialized.selector);
        adapter.swapExactIn(_one(ghost), address(usdcA), 1 ether, 0, bob);
        vm.stopPrank();
    }

    function test_UnregisteredPoolManagerReverts() public {
        PoolKey memory fake = _pool(address(0), address(usdcA), otherHooks, roguePm, 3 * ONE);
        vm.prank(alice);
        vm.expectRevert(abi.encodeWithSelector(InfinitySwapAdapter.UnknownPoolManager.selector, address(roguePm)));
        adapter.swapExactIn{value: 1 ether}(_one(fake), address(0), 1 ether, 0, bob);
    }

    function test_PartialFillReverts() public {
        pm.setMaxIn(routeNative, 0.5 ether);
        vm.prank(alice);
        vm.expectRevert(abi.encodeWithSelector(InfinitySwapAdapter.PartialFill.selector, 0.5 ether, 1 ether));
        adapter.swapExactIn{value: 1 ether}(_one(routeNative), address(0), 1 ether, 0, bob);
    }

    function test_LockAcquiredRejectsNonVault() public {
        vm.prank(alice);
        vm.expectRevert(InfinitySwapAdapter.NotVault.selector);
        adapter.lockAcquired("");
    }

    function test_QuoteOfPausedPoolBubblesUp() public {
        pm.setPaused(poolA, true);
        vm.expectRevert(MockCLPoolManager.PoolPaused.selector);
        adapter.quoteExactIn(_one(poolA), address(usdcA), 1 ether);
    }

    function testFuzz_SwapMatchesQuoteAndMinOut(uint256 amountIn) public {
        // Output is 6x the input and the vault holds 1e30 tokenA, so keep the payout within reach.
        amountIn = bound(amountIn, 1, 1e28);
        vm.deal(alice, amountIn);
        PoolKey[] memory path = _two(routeNative, poolA);

        uint256 quoted = adapter.quoteExactIn(path, address(0), amountIn);
        assertEq(quoted, amountIn * 3 * 2, "quote");

        // One above the quote must revert and roll back.
        vm.prank(alice);
        vm.expectRevert(abi.encodeWithSelector(InfinitySwapAdapter.InsufficientOutput.selector, quoted, quoted + 1));
        adapter.swapExactIn{value: amountIn}(path, address(0), amountIn, quoted + 1, bob);
        assertEq(tokenA.balanceOf(bob), 0);

        vm.prank(alice);
        uint256 out = adapter.swapExactIn{value: amountIn}(path, address(0), amountIn, quoted, bob);
        assertEq(out, quoted);
        assertEq(tokenA.balanceOf(bob), quoted);
        _assertAdapterEmpty();
    }
}
