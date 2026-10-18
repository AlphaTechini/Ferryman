/**
 * Minimal ABIs for the live service layer, copied from the Solidity sources:
 *  - contracts/src/elysium/FerryIn.sol          (buy)
 *  - contracts/src/elysium/ExitDesk.sol         (previewReceiver, exitHYPE, ExitIntent)
 *  - contracts/src/libraries/ExitParams.sol     (ExitParams field order)
 *  - Arbitrum Inbox (docs/RESEARCH.md 3.1)      (createRetryableTicket, calculateRetryableSubmissionFee)
 */

export const exitParamsComponents = [
  { name: 'chainId', type: 'uint256' },
  { name: 'desk', type: 'address' },
  { name: 'sender', type: 'address' },
  { name: 'nonce', type: 'uint256' },
  { name: 'amount', type: 'uint256' },
  { name: 'claimTip', type: 'uint256' },
  { name: 'recipient', type: 'address' },
  { name: 'toCore', type: 'bool' },
] as const;

/** FerryIn on Elysium. */
export const ferryInAbi = [
  {
    type: 'function',
    name: 'buy',
    stateMutability: 'payable',
    inputs: [
      { name: 'token', type: 'address' },
      { name: 'recipient', type: 'address' },
      { name: 'minOut', type: 'uint256' },
    ],
    outputs: [],
  },
] as const;

/** ExitDesk on Elysium. */
export const exitDeskAbi = [
  {
    type: 'function',
    name: 'previewReceiver',
    stateMutability: 'view',
    inputs: [
      { name: 'sender', type: 'address' },
      { name: 'recipient', type: 'address' },
      { name: 'amount', type: 'uint256' },
      { name: 'claimTip', type: 'uint256' },
      { name: 'toCore', type: 'bool' },
    ],
    outputs: [
      { name: 'params', type: 'tuple', components: exitParamsComponents },
      { name: 'receiver', type: 'address' },
    ],
  },
  {
    type: 'function',
    name: 'exitHYPE',
    stateMutability: 'payable',
    inputs: [
      { name: 'recipient', type: 'address' },
      { name: 'claimTip', type: 'uint256' },
      { name: 'toCore', type: 'bool' },
    ],
    outputs: [{ name: 'receiver', type: 'address' }],
  },
  {
    type: 'function',
    name: 'nonces',
    stateMutability: 'view',
    inputs: [{ name: 'sender', type: 'address' }],
    outputs: [{ name: '', type: 'uint256' }],
  },
  {
    type: 'event',
    name: 'ExitIntent',
    anonymous: false,
    inputs: [
      { name: 'salt', type: 'bytes32', indexed: true },
      { name: 'receiver', type: 'address', indexed: true },
      { name: 'recipient', type: 'address', indexed: true },
      { name: 'params', type: 'tuple', indexed: false, components: exitParamsComponents },
    ],
  },
] as const;

/** Arbitrum Nitro Inbox on HyperEVM (ETH-native / payable variant). */
export const inboxAbi = [
  {
    type: 'function',
    name: 'createRetryableTicket',
    stateMutability: 'payable',
    inputs: [
      { name: 'to', type: 'address' },
      { name: 'l2CallValue', type: 'uint256' },
      { name: 'maxSubmissionCost', type: 'uint256' },
      { name: 'excessFeeRefundAddress', type: 'address' },
      { name: 'callValueRefundAddress', type: 'address' },
      { name: 'gasLimit', type: 'uint256' },
      { name: 'maxFeePerGas', type: 'uint256' },
      { name: 'data', type: 'bytes' },
    ],
    outputs: [{ name: '', type: 'uint256' }],
  },
  {
    type: 'function',
    name: 'calculateRetryableSubmissionFee',
    stateMutability: 'view',
    inputs: [
      { name: 'dataLength', type: 'uint256' },
      { name: 'baseFee', type: 'uint256' },
    ],
    outputs: [{ name: '', type: 'uint256' }],
  },
] as const;
