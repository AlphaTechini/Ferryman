// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

import {Script, console2} from "forge-std/Script.sol";
import {PoolKey} from "../src/libraries/InfinityTypes.sol";
import {Receiver} from "../src/hyperevm/Receiver.sol";
import {ExitDesk} from "../src/elysium/ExitDesk.sol";
import {FeeJar} from "../src/elysium/FeeJar.sol";
import {InfinitySwapAdapter} from "../src/elysium/InfinitySwapAdapter.sol";
import {FerryIn} from "../src/elysium/FerryIn.sol";
import {WHYPE9} from "../src/elysium/WHYPE9.sol";

/// @notice Deploys the Elysium side: FeeJar, ExitDesk, InfinitySwapAdapter, FerryIn (and WHYPE9 if asked).
/// @dev Env:
///   EXIT_FACTORY        (required) ExitFactory address from DeployHyperEVM.
///   FEE_BENEFICIARY     (required) FeeJar beneficiary (fee router).
///   HYPE_ROUTES_JSON    (required) Path to the HYPE route JSON. Shape: docs in contracts/README.md and
///                       deployments/hype-routes.example.json.
///   RECEIVER_INIT_HASH  (optional) Checked against the locally computed hash. Mismatch reverts.
///   VAULT, LAUNCHPAD, WHYPE (optional) Default to the Elysium testnet addresses in docs/INTERFACES.md / details.md.
///   DEPLOY_WHYPE9       (optional, default false) Deploys WHYPE9 and uses it instead of WHYPE.
///   Key comes from `--private-key` or `--account`. The script never reads PRIVATE_KEY.
contract DeployElysium is Script {
    address internal constant DEFAULT_VAULT = 0x2B738A9A69b7a45C9dA6B519f6bE6efBfb912aec;
    address internal constant DEFAULT_LAUNCHPAD = 0x6d01f6644bbf1b5074472D3e85aa1465B4d2bF3d;
    address internal constant DEFAULT_WHYPE = 0xCD57F65c2B0e5881CFC2E609F7CD53b746E1F234;

    function run() external returns (FeeJar feeJar, ExitDesk desk, InfinitySwapAdapter adapter, FerryIn ferryIn) {
        address exitFactory = vm.envAddress("EXIT_FACTORY");
        bytes32 receiverInitHash = receiverInitHashOf(exitFactory);
        if (vm.envOr("RECEIVER_INIT_HASH", bytes32(0)) != bytes32(0)) {
            require(vm.envBytes32("RECEIVER_INIT_HASH") == receiverInitHash, "RECEIVER_INIT_HASH mismatch");
        }

        address feeBeneficiary = vm.envAddress("FEE_BENEFICIARY");
        address vault = vm.envOr("VAULT", DEFAULT_VAULT);
        address launchpad = vm.envOr("LAUNCHPAD", DEFAULT_LAUNCHPAD);
        address whype = vm.envOr("WHYPE", DEFAULT_WHYPE);
        bool deployWhype9 = vm.envOr("DEPLOY_WHYPE9", false);
        PoolKey[] memory routes = readRoutes(vm.envString("HYPE_ROUTES_JSON"));

        vm.startBroadcast();
        if (deployWhype9) whype = address(new WHYPE9());
        feeJar = new FeeJar(feeBeneficiary);
        desk = new ExitDesk(exitFactory, receiverInitHash);
        adapter = new InfinitySwapAdapter(vault, whype);
        ferryIn = new FerryIn(address(adapter), address(feeJar), launchpad, whype, routes);
        vm.stopBroadcast();

        console2.log("WHYPE", whype);
        console2.log("FeeJar", address(feeJar));
        console2.log("ExitDesk", address(desk));
        console2.log("InfinitySwapAdapter", address(adapter));
        console2.log("FerryIn", address(ferryIn));
        console2.log("ExitFactory (HyperEVM, for reference)", exitFactory);
        console2.log("receiverInitHash:");
        console2.logBytes32(receiverInitHash);
    }

    /// @notice keccak256(Receiver creation code ++ abi.encode(factory)), the same value ExitFactory computes.
    function receiverInitHashOf(address exitFactory) public pure returns (bytes32) {
        return keccak256(abi.encodePacked(type(Receiver).creationCode, abi.encode(exitFactory)));
    }

    /// @notice Reads `{"routes": [{currency0, currency1, hooks, poolManager, fee, parameters}, ...]}`.
    function readRoutes(string memory path) public view returns (PoolKey[] memory routes) {
        string memory json = vm.readFile(path);
        uint256 n = 0;
        while (vm.keyExistsJson(json, string.concat(".routes[", vm.toString(n), "].currency0"))) n++;
        require(n > 0, "no routes in HYPE_ROUTES_JSON");

        routes = new PoolKey[](n);
        for (uint256 i = 0; i < n; ++i) {
            string memory p = string.concat(".routes[", vm.toString(i), "]");
            routes[i] = PoolKey({
                currency0: vm.parseJsonAddress(json, string.concat(p, ".currency0")),
                currency1: vm.parseJsonAddress(json, string.concat(p, ".currency1")),
                hooks: vm.parseJsonAddress(json, string.concat(p, ".hooks")),
                poolManager: vm.parseJsonAddress(json, string.concat(p, ".poolManager")),
                // forge-lint: disable-next-line(unsafe-typecast)
                fee: uint24(vm.parseJsonUint(json, string.concat(p, ".fee"))),
                parameters: vm.parseJsonBytes32(json, string.concat(p, ".parameters"))
            });
        }
    }
}
