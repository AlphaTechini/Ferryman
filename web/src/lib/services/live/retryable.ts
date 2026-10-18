import type { Quote } from '../types';

/** Multiplier applied to Inbox.calculateRetryableSubmissionFee when the keeper gives no submission cost. */
export const SUBMISSION_COST_MARGIN = 4n;

export type RetryableParams = {
  gasLimit: bigint;
  maxFeePerGas: bigint;
  maxSubmissionCost: bigint;
  /** msg.value for Inbox.createRetryableTicket, always recomputed locally. */
  totalValue: bigint;
  /** True when the keeper's totalValue (or submission cost) disagreed with the local math. */
  recomputed: boolean;
};

/** msg.value the Inbox requires: l2CallValue + maxSubmissionCost + gasLimit * maxFeePerGas. */
export function retryableTotal(
  l2CallValue: bigint,
  maxSubmissionCost: bigint,
  gasLimit: bigint,
  maxFeePerGas: bigint,
): bigint {
  return l2CallValue + maxSubmissionCost + gasLimit * maxFeePerGas;
}

/** True when the keeper did not supply a usable maxSubmissionCost, so the caller must read the Inbox. */
export function needsSubmissionFee(retryable: Quote['retryable']): boolean {
  return !retryable.maxSubmissionCost || retryable.maxSubmissionCost === '0';
}

/**
 * Resolves the retryable parameters for a Ferry In ticket.
 * `minSubmissionFee` is Inbox.calculateRetryableSubmissionFee(dataLength, 0); it is only needed
 * when `needsSubmissionFee` is true and is multiplied by SUBMISSION_COST_MARGIN.
 */
export function resolveRetryable(
  retryable: Quote['retryable'],
  amountWei: bigint,
  minSubmissionFee?: bigint,
): RetryableParams {
  const gasLimit = toBig(retryable.gasLimit, 'gasLimit');
  const maxFeePerGas = toBig(retryable.maxFeePerGas, 'maxFeePerGas');
  if (gasLimit <= 1n || maxFeePerGas <= 1n) throw new Error('Quote has invalid retryable gas parameters');

  let maxSubmissionCost: bigint;
  if (needsSubmissionFee(retryable)) {
    if (minSubmissionFee === undefined) throw new Error('Missing retryable submission cost');
    maxSubmissionCost = minSubmissionFee * SUBMISSION_COST_MARGIN;
  } else {
    maxSubmissionCost = toBig(retryable.maxSubmissionCost, 'maxSubmissionCost');
  }

  const totalValue = retryableTotal(amountWei, maxSubmissionCost, gasLimit, maxFeePerGas);
  const keeperTotal = retryable.totalValue ? BigInt(retryable.totalValue) : null;
  return {
    gasLimit,
    maxFeePerGas,
    maxSubmissionCost,
    totalValue,
    recomputed: keeperTotal !== totalValue,
  };
}

function toBig(value: string | undefined, field: string): bigint {
  if (!value) throw new Error(`Quote is missing retryable ${field}`);
  return BigInt(value);
}
