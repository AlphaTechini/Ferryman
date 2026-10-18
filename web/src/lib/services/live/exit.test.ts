import { describe, expect, it } from 'vitest';
import { toExitParams } from './exit';

describe('toExitParams', () => {
  it('maps the decoded ExitDesk struct to UI types', () => {
    const params = toExitParams({
      chainId: 99801n,
      desk: '0x2222222222222222222222222222222222222222',
      sender: '0x3333333333333333333333333333333333333333',
      nonce: 7n,
      amount: 10n ** 18n,
      claimTip: 500_000_000_000_000n,
      recipient: '0x4444444444444444444444444444444444444444',
      toCore: true,
    });

    expect(params).toEqual({
      chainId: 99801,
      desk: '0x2222222222222222222222222222222222222222',
      sender: '0x3333333333333333333333333333333333333333',
      nonce: '7',
      amount: '1000000000000000000',
      claimTip: '500000000000000',
      recipient: '0x4444444444444444444444444444444444444444',
      toCore: true,
    });
  });
});
