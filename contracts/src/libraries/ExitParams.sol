// SPDX-License-Identifier: MIT
pragma solidity 0.8.28;

/// @notice Parameters of one Ferry Out exit. Shared by ExitDesk (Elysium) and ExitFactory (HyperEVM).
/// @dev Field order is part of the interface (see docs/INTERFACES.md 2.1). Do not reorder.
struct ExitParams {
    uint256 chainId; // Elysium chain id (99801 on testnet)
    address desk; // ExitDesk that started the exit
    address sender; // msg.sender of exitHYPE
    uint256 nonce; // per-sender nonce on the desk (0, 1, 2, ...)
    uint256 amount; // HYPE sent to ArbSys.withdrawEth
    uint256 claimTip; // paid to whoever runs claimAndSweep, <= amount
    address recipient; // HyperEVM address that receives amount - claimTip
    bool toCore; // reserved for the UI; contracts always pay recipient on HyperEVM
}

library ExitParamsLib {
    /// @notice Salt that binds the receiver address to every field of the exit.
    function salt(ExitParams memory p) internal pure returns (bytes32) {
        return keccak256(abi.encode(p));
    }

    /// @notice Standard CREATE2 address of a Receiver deployed by `factory` with `salt_`.
    function computeReceiver(address factory, bytes32 salt_, bytes32 initHash) internal pure returns (address) {
        return address(uint160(uint256(keccak256(abi.encodePacked(bytes1(0xff), factory, salt_, initHash)))));
    }
}
