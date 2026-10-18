// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

import {Test} from "forge-std/Test.sol";
import {ExitParams, ExitParamsLib} from "../src/libraries/ExitParams.sol";
import {IOutbox} from "../src/interfaces/IOutbox.sol";
import {ExitDesk} from "../src/elysium/ExitDesk.sol";
import {ExitFactory} from "../src/hyperevm/ExitFactory.sol";
import {MockArbSys} from "./mocks/MockArbSys.sol";
import {MockOutbox} from "./mocks/MockOutbox.sol";

/// @dev Shared setup: ArbSys mock at 0x64, Outbox mock, ExitFactory (HyperEVM) and ExitDesk (Elysium).
///      Both chains live in one test EVM; vm.chainId switches between them.
abstract contract FerryFixture is Test {
    address internal constant ARB_SYS = address(0x64);
    uint256 internal constant ELYSIUM_CHAIN_ID = 99801;
    uint256 internal constant HYPEREVM_CHAIN_ID = 998;

    MockArbSys internal arbSys;
    MockOutbox internal outbox;
    ExitFactory internal factory;
    ExitDesk internal desk;

    address internal alice = makeAddr("alice"); // exit sender on Elysium
    address internal bob = makeAddr("bob"); // HyperEVM recipient
    address internal keeper = makeAddr("keeper"); // runs claimAndSweep

    function setUp() public virtual {
        vm.chainId(HYPEREVM_CHAIN_ID);
        vm.etch(ARB_SYS, type(MockArbSys).runtimeCode);
        arbSys = MockArbSys(payable(ARB_SYS));
        outbox = new MockOutbox();
        factory = new ExitFactory(address(outbox));

        vm.chainId(ELYSIUM_CHAIN_ID);
        desk = new ExitDesk(address(factory), factory.receiverInitHash());
        vm.chainId(HYPEREVM_CHAIN_ID);

        vm.deal(alice, 1_000 ether);
    }

    function _onElysium() internal {
        vm.chainId(ELYSIUM_CHAIN_ID);
    }

    function _onHyperEVM() internal {
        vm.chainId(HYPEREVM_CHAIN_ID);
    }

    /// @dev Runs exitHYPE on Elysium and returns the params and receiver it produced.
    function _exit(address sender, address recipient, uint256 amount, uint256 tip)
        internal
        returns (ExitParams memory p, address receiver)
    {
        _onElysium();
        (p, receiver) = desk.previewReceiver(sender, recipient, amount, tip, false);
        vm.prank(sender);
        address got = desk.exitHYPE{value: amount}(recipient, tip, false);
        assertEq(got, receiver, "exitHYPE receiver != preview");
        _onHyperEVM();
    }

    /// @dev Outbox calldata as the Arbitrum claim endpoint would return it: `to` is the receiver.
    function _claimCalldata(address to, uint256 value, uint256 index) internal view returns (bytes memory) {
        bytes32[] memory proof = new bytes32[](0);
        return abi.encodeCall(
            IOutbox.executeTransaction, (proof, index, address(desk), to, 1, 2, block.timestamp, value, new bytes(0))
        );
    }

    /// @dev Sends HYPE to any address (works for undeployed receivers too).
    function _fund(address to, uint256 amount) internal {
        vm.deal(address(this), address(this).balance + amount);
        (bool ok,) = to.call{value: amount}("");
        require(ok, "fund failed");
    }

    receive() external payable {}
}
