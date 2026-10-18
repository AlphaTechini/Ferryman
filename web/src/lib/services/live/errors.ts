import { UserRejectedRequestError } from 'viem';
import { UserRejectedError } from '../types';

/** True for EIP-1193 code 4001 or viem's UserRejectedRequestError, anywhere in the cause chain. */
export function isUserRejection(err: unknown): boolean {
  let current: unknown = err;
  for (let depth = 0; depth < 6 && current; depth++) {
    if (current instanceof UserRejectedRequestError) return true;
    if (typeof current === 'object' && 'code' in current && current.code === 4001) return true;
    if (current instanceof Error && /user (rejected|denied)/i.test(current.message)) return true;
    current = typeof current === 'object' && 'cause' in current ? current.cause : undefined;
  }
  return false;
}

/** EIP-3085: the wallet does not know the chain yet (4902, or -32603 with "Unrecognized chain" from some wallets). */
export function isUnknownChain(err: unknown): boolean {
  let current: unknown = err;
  for (let depth = 0; depth < 6 && current; depth++) {
    if (typeof current === 'object' && 'code' in current && current.code === 4902) return true;
    if (current instanceof Error && /unrecognized chain/i.test(current.message)) return true;
    current = typeof current === 'object' && 'cause' in current ? current.cause : undefined;
  }
  return false;
}

/** Runs a wallet call and turns user rejection into UserRejectedError. Other errors pass through. */
export async function guard<T>(fn: () => Promise<T>): Promise<T> {
  try {
    return await fn();
  } catch (err) {
    if (isUserRejection(err)) throw new UserRejectedError();
    throw err;
  }
}
