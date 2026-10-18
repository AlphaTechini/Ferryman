import { describe, expect, it } from 'vitest';
import { needsSubmissionFee, resolveRetryable, retryableTotal } from './retryable';

const ONE_HYPE = 10n ** 18n;

describe('retryableTotal', () => {
  it('is l2CallValue + maxSubmissionCost + gasLimit * maxFeePerGas', () => {
    expect(retryableTotal(ONE_HYPE, 100n, 1_000_000n, 2n)).toBe(ONE_HYPE + 100n + 2_000_000n);
  });
});

describe('resolveRetryable', () => {
  const keeper = {
    gasLimit: '1250000',
    maxFeePerGas: '100000000',
    maxSubmissionCost: '21400000000000',
    totalValue: '0',
  };

  it('uses the keeper values and recomputes totalValue when the keeper total is wrong', () => {
    const r = resolveRetryable(keeper, ONE_HYPE);
    expect(r.gasLimit).toBe(1_250_000n);
    expect(r.maxFeePerGas).toBe(100_000_000n);
    expect(r.maxSubmissionCost).toBe(21_400_000_000_000n);
    expect(r.totalValue).toBe(ONE_HYPE + 21_400_000_000_000n + 1_250_000n * 100_000_000n);
    expect(r.recomputed).toBe(true);
  });

  it('is not flagged when the keeper total matches', () => {
    const total = ONE_HYPE + 21_400_000_000_000n + 1_250_000n * 100_000_000n;
    const r = resolveRetryable({ ...keeper, totalValue: total.toString() }, ONE_HYPE);
    expect(r.totalValue).toBe(total);
    expect(r.recomputed).toBe(false);
  });

  it('reads the submission fee x4 when the keeper gives 0 or nothing', () => {
    expect(needsSubmissionFee({ ...keeper, maxSubmissionCost: '0' })).toBe(true);
    expect(needsSubmissionFee({ ...keeper, maxSubmissionCost: '' })).toBe(true);
    expect(needsSubmissionFee(keeper)).toBe(false);

    const r = resolveRetryable({ ...keeper, maxSubmissionCost: '0' }, ONE_HYPE, 5_000n);
    expect(r.maxSubmissionCost).toBe(20_000n);
    expect(r.totalValue).toBe(ONE_HYPE + 20_000n + 1_250_000n * 100_000_000n);
  });

  it('refuses to resolve without a submission fee when the keeper gave none', () => {
    expect(() => resolveRetryable({ ...keeper, maxSubmissionCost: '0' }, ONE_HYPE)).toThrow(
      'Missing retryable submission cost',
    );
  });

  it('rejects gas values of 1 or missing values', () => {
    expect(() => resolveRetryable({ ...keeper, gasLimit: '1' }, ONE_HYPE)).toThrow();
    expect(() => resolveRetryable({ ...keeper, maxFeePerGas: '' }, ONE_HYPE)).toThrow('maxFeePerGas');
  });
});
