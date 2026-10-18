// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

import {FerryFixture} from "./FerryFixture.sol";
import {ExitParams, ExitParamsLib} from "../src/libraries/ExitParams.sol";
import {IOutbox} from "../src/interfaces/IOutbox.sol";
import {Receiver} from "../src/hyperevm/Receiver.sol";

contract ExitParamsTest is FerryFixture {
    function test_OutboxSelectorMatchesArbitrum() public pure {
        assertEq(IOutbox.executeTransaction.selector, bytes4(0x08635a95));
    }

    function test_SaltIsHashOfAbiEncodedParams() public view {
        ExitParams memory p = _sample();
        assertEq(ExitParamsLib.salt(p), keccak256(abi.encode(p)));
    }

    function test_SaltChangesWithEveryField() public view {
        ExitParams memory base = _sample();
        bytes32 s = ExitParamsLib.salt(base);

        ExitParams memory q = _sample();
        q.chainId += 1;
        assertTrue(ExitParamsLib.salt(q) != s, "chainId");
        q = _sample();
        q.desk = address(0xD35C);
        assertTrue(ExitParamsLib.salt(q) != s, "desk");
        q = _sample();
        q.sender = address(0x5E4D);
        assertTrue(ExitParamsLib.salt(q) != s, "sender");
        q = _sample();
        q.nonce += 1;
        assertTrue(ExitParamsLib.salt(q) != s, "nonce");
        q = _sample();
        q.amount += 1;
        assertTrue(ExitParamsLib.salt(q) != s, "amount");
        q = _sample();
        q.claimTip += 1;
        assertTrue(ExitParamsLib.salt(q) != s, "claimTip");
        q = _sample();
        q.recipient = address(0xBEEF);
        assertTrue(ExitParamsLib.salt(q) != s, "recipient");
        q = _sample();
        q.toCore = !q.toCore;
        assertTrue(ExitParamsLib.salt(q) != s, "toCore");
    }

    function test_ComputeReceiverIsStandardCreate2() public view {
        ExitParams memory p = _sample();
        bytes32 salt = ExitParamsLib.salt(p);
        bytes32 initHash = keccak256(abi.encodePacked(type(Receiver).creationCode, abi.encode(address(factory))));
        address expected = vm.computeCreate2Address(salt, initHash, address(factory));
        assertEq(ExitParamsLib.computeReceiver(address(factory), salt, initHash), expected);
    }

    function test_ReceiverInitHashFormula() public view {
        bytes32 expected = keccak256(abi.encodePacked(type(Receiver).creationCode, abi.encode(address(factory))));
        assertEq(factory.receiverInitHash(), expected);
        assertEq(desk.receiverInitHash(), expected);
        assertEq(desk.exitFactory(), address(factory));
    }

    function _sample() internal view returns (ExitParams memory) {
        return ExitParams({
            chainId: ELYSIUM_CHAIN_ID,
            desk: address(desk),
            sender: alice,
            nonce: 0,
            amount: 5 ether,
            claimTip: 0.05 ether,
            recipient: bob,
            toCore: false
        });
    }
}
