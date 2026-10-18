// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

import {Script, console2} from "forge-std/Script.sol";
import {ExitFactory} from "../src/hyperevm/ExitFactory.sol";

/// @notice Deploys ExitFactory on HyperEVM. Run this first; its address goes into EXIT_FACTORY for DeployElysium.
/// @dev Key comes from `--private-key` or `--account`. Example:
///      forge script script/DeployHyperEVM.s.sol --rpc-url https://rpc.hyperliquid-testnet.xyz/evm --account <name> --broadcast
contract DeployHyperEVM is Script {
    address internal constant DEFAULT_OUTBOX = 0x87391D602aaffB3Fe8051EcCDb75d6EEe8C31060;

    function run() external returns (ExitFactory factory) {
        address outbox = vm.envOr("OUTBOX", DEFAULT_OUTBOX);

        vm.startBroadcast();
        factory = new ExitFactory(outbox);
        vm.stopBroadcast();

        console2.log("ExitFactory", address(factory));
        console2.log("Outbox", outbox);
        console2.log("receiverInitHash:");
        console2.logBytes32(factory.receiverInitHash());
    }
}
