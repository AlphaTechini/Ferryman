// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

import {Test} from "forge-std/Test.sol";
import {DeployElysium} from "../script/DeployElysium.s.sol";
import {ExitFactory} from "../src/hyperevm/ExitFactory.sol";
import {PoolKey} from "../src/libraries/InfinityTypes.sol";
import {MockVault} from "./mocks/MockVault.sol";
import {MockCLPoolManager} from "./mocks/MockCLPoolManager.sol";

/// @dev Checks the pure helpers the deploy scripts rely on. Full broadcast is not run here.
contract DeployScriptsTest is Test {
    DeployElysium internal script;

    function setUp() public {
        script = new DeployElysium();
    }

    function test_ReceiverInitHashMatchesExitFactory() public {
        address outbox = makeAddr("outbox");
        address factoryAddr = makeAddr("factory");
        ExitFactory factory = new ExitFactory(outbox);
        // ExitFactory hashes its own address, so compare the helper at that same address.
        vm.etch(factoryAddr, address(factory).code);
        assertEq(script.receiverInitHashOf(address(factory)), factory.receiverInitHash());
    }

    function test_ReadRoutesParsesExampleFile() public view {
        PoolKey[] memory routes = script.readRoutes("/home/user/Ferryman/deployments/hype-routes.example.json");
        assertEq(routes.length, 2);
        assertEq(routes[0].currency0, address(0));
        assertEq(routes[0].currency1, address(0x1111111111111111111111111111111111111111));
        assertEq(routes[0].poolManager, address(0x2222222222222222222222222222222222222222));
        assertEq(routes[0].fee, 3000);
        assertEq(routes[0].parameters, bytes32(uint256(0x3c00)));
        assertEq(routes[1].currency0, 0xCD57F65c2B0e5881CFC2E609F7CD53b746E1F234);
    }

    /// @dev Runs the whole deploy script against the example routes file. The route pools must exist,
    ///      so a mock pool manager is placed at the example's poolManager address with both pools initialized.
    function test_RunDeploysElysiumStack() public {
        address poolManagerAddr = 0x2222222222222222222222222222222222222222;
        MockVault vault = new MockVault();
        deployCodeTo("MockCLPoolManager.sol:MockCLPoolManager", abi.encode(address(vault)), poolManagerAddr);
        MockCLPoolManager pm = MockCLPoolManager(poolManagerAddr);
        pm.initialize(
            PoolKey({
                currency0: address(0),
                currency1: address(0x1111111111111111111111111111111111111111),
                hooks: address(0),
                poolManager: poolManagerAddr,
                fee: 3000,
                parameters: bytes32(uint256(0x3c00))
            }),
            3e18
        );
        pm.initialize(
            PoolKey({
                currency0: 0xCD57F65c2B0e5881CFC2E609F7CD53b746E1F234,
                currency1: address(0x3333333333333333333333333333333333333333),
                hooks: address(0),
                poolManager: poolManagerAddr,
                fee: 3000,
                parameters: bytes32(uint256(0x3c00))
            }),
            2e18
        );

        address factory = makeAddr("exitFactory");
        vm.setEnv("EXIT_FACTORY", vm.toString(factory));
        vm.setEnv("FEE_BENEFICIARY", vm.toString(makeAddr("feeRouter")));
        vm.setEnv("HYPE_ROUTES_JSON", "/home/user/Ferryman/deployments/hype-routes.example.json");
        vm.setEnv("VAULT", vm.toString(address(vault)));

        script.run();
    }
}
