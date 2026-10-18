import type { Address, ExitParams } from '../types';

/** Raw ExitParams as decoded by viem from ExitDesk (bigints, see contracts/src/libraries/ExitParams.sol). */
export type RawExitParams = {
  chainId: bigint;
  desk: Address;
  sender: Address;
  nonce: bigint;
  amount: bigint;
  claimTip: bigint;
  recipient: Address;
  toCore: boolean;
};

/** Maps a decoded ExitParams struct to the UI type: numbers stay numbers, uint256 become decimal strings. */
export function toExitParams(p: RawExitParams): ExitParams {
  return {
    chainId: Number(p.chainId),
    desk: p.desk,
    sender: p.sender,
    nonce: p.nonce.toString(),
    amount: p.amount.toString(),
    claimTip: p.claimTip.toString(),
    recipient: p.recipient,
    toCore: p.toCore,
  };
}
