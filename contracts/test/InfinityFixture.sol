// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

import {Test} from "forge-std/Test.sol";
import {PoolKey} from "../src/libraries/InfinityTypes.sol";
import {InfinitySwapAdapter} from "../src/elysium/InfinitySwapAdapter.sol";
import {FerryIn} from "../src/elysium/FerryIn.sol";
import {FeeJar} from "../src/elysium/FeeJar.sol";
import {WHYPE9} from "../src/elysium/WHYPE9.sol";
import {MockVault} from "./mocks/MockVault.sol";
import {MockCLPoolManager} from "./mocks/MockCLPoolManager.sol";
import {MockERC20} from "./mocks/MockERC20.sol";

/// @dev Shared setup for swap and Ferry In tests. Pools are constant-rate; `_pool` picks the rate
///      so the price reads the same in either direction (e.g. 2e18 means 2 currencyOut per currencyIn).
abstract contract InfinityFixture is Test {
    uint256 internal constant ONE = 1e18;
    uint24 internal constant FEE = 3000;
    bytes32 internal constant PARAMS = bytes32(uint256(60) << 16);

    MockVault internal vault;
    MockCLPoolManager internal pm;
    MockCLPoolManager internal roguePm; // never registered with the vault
    WHYPE9 internal whype;
    MockERC20 internal usdcA;
    MockERC20 internal usdcB;
    MockERC20 internal tokenA; // launched by us, registered in setUp
    MockERC20 internal tokenB; // launched by us, quote usdcB, registered in setUp
    MockERC20 internal tokenC; // launched by carol, registered by her in tests

    address internal launchpad = makeAddr("launchpad");
    address internal otherHooks = makeAddr("otherHooks");
    address internal alice = makeAddr("alice");
    address internal bob = makeAddr("bob");
    address internal carol = makeAddr("carol");
    address internal feeRouter = makeAddr("feeRouter");

    PoolKey internal routeNative; // HYPE (native) -> usdcA, 3 usdcA per HYPE
    PoolKey internal routeWhype; // WHYPE -> usdcB, 2 usdcB per WHYPE
    PoolKey internal poolA; // usdcA -> tokenA, 2 tokenA per usdcA
    PoolKey internal poolB; // usdcB -> tokenB, 4 tokenB per usdcB
    PoolKey internal poolC; // usdcA -> tokenC, 0.5 tokenC per usdcA

    InfinitySwapAdapter internal adapter;
    FeeJar internal feeJar;
    FerryIn internal ferry;

    function setUp() public virtual {
        vault = new MockVault();
        pm = new MockCLPoolManager(vault);
        roguePm = new MockCLPoolManager(vault);
        vault.registerApp(address(pm));

        whype = new WHYPE9();
        usdcA = new MockERC20("Mock USDC A", "mUSDC-A", 18);
        usdcB = new MockERC20("Mock USDC B", "mUSDC-B", 18);
        tokenA = new MockERC20("Launch A", "LA", 18);
        tokenB = new MockERC20("Launch B", "LB", 18);
        tokenC = new MockERC20("Launch C", "LC", 18);

        // Liquidity for payouts. The adapter's settlements also land here.
        vm.deal(address(vault), 1_000 ether);
        usdcA.mint(address(vault), 1e30);
        usdcB.mint(address(vault), 1e30);
        tokenA.mint(address(vault), 1e30);
        tokenB.mint(address(vault), 1e30);
        tokenC.mint(address(vault), 1e30);

        routeNative = _pool(address(0), address(usdcA), otherHooks, pm, 3 * ONE);
        routeWhype = _pool(address(whype), address(usdcB), otherHooks, pm, 2 * ONE);
        poolA = _pool(address(usdcA), address(tokenA), launchpad, pm, 2 * ONE);
        poolB = _pool(address(usdcB), address(tokenB), launchpad, pm, 4 * ONE);
        poolC = _pool(address(usdcA), address(tokenC), launchpad, pm, ONE / 2);

        adapter = new InfinitySwapAdapter(address(vault), address(whype));
        feeJar = new FeeJar(feeRouter);
        ferry = _deployFerry(address(feeJar));
        ferry.registerToken(poolA);
        ferry.registerToken(poolB);

        vm.deal(alice, 1_000 ether);
        usdcA.mint(alice, 1e24);
        usdcB.mint(alice, 1e24);
        tokenA.mint(alice, 1e24);
    }

    function _deployFerry(address feeJar_) internal returns (FerryIn) {
        PoolKey[] memory routes = new PoolKey[](2);
        routes[0] = routeNative;
        routes[1] = routeWhype;
        return new FerryIn(address(adapter), feeJar_, launchpad, address(whype), routes);
    }

    /// @dev Sorted pool key (currency0 < currency1), like infinity-core requires.
    function _key(address a, address b, address hooks, MockCLPoolManager manager)
        internal
        pure
        returns (PoolKey memory key)
    {
        (address lo, address hi) = a < b ? (a, b) : (b, a);
        key = PoolKey({
            currency0: lo, currency1: hi, hooks: hooks, poolManager: address(manager), fee: FEE, parameters: PARAMS
        });
    }

    /// @dev Builds and initializes a pool so that `currencyIn` buys `outPerIn` (1e18 scale) of `currencyOut`.
    function _pool(address currencyIn, address currencyOut, address hooks, MockCLPoolManager manager, uint256 outPerIn)
        internal
        returns (PoolKey memory key)
    {
        key = _key(currencyIn, currencyOut, hooks, manager);
        // The stored rate is currency1 per currency0. Invert it when the input is currency1.
        uint256 rate = currencyIn == key.currency0 ? outPerIn : 1e36 / outPerIn;
        manager.initialize(key, rate);
    }

    function _one(PoolKey memory a) internal pure returns (PoolKey[] memory path) {
        path = new PoolKey[](1);
        path[0] = a;
    }

    function _two(PoolKey memory a, PoolKey memory b) internal pure returns (PoolKey[] memory path) {
        path = new PoolKey[](2);
        path[0] = a;
        path[1] = b;
    }

    receive() external payable {}
}
