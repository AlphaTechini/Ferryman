// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

/// @dev Minimal Outbox: pays `value` to `to` from its own balance, once per `index`.
contract MockOutbox {
    mapping(uint256 => bool) public spent;

    function executeTransaction(
        bytes32[] calldata,
        uint256 index,
        address,
        address to,
        uint256,
        uint256,
        uint256,
        uint256 value,
        bytes calldata
    ) external {
        require(!spent[index], "ALREADY_SPENT");
        spent[index] = true;
        (bool ok,) = to.call{value: value}("");
        require(ok, "TRANSFER_FAILED");
    }
}
