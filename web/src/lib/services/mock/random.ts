import { getAddress } from 'viem';
import type { Address, Hash } from '../types';
export { hashString } from '../../util/hash';

/** Small deterministic PRNG (mulberry32) so demo data is stable across reloads. */
export function rng(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function hex(bytes: number, next: () => number): string {
  let out = '0x';
  for (let i = 0; i < bytes; i++) out += Math.floor(next() * 256).toString(16).padStart(2, '0');
  return out;
}

export const randomAddress = (next: () => number): Address => getAddress(hex(20, next));
export const randomHash = (next: () => number): Hash => hex(32, next) as Hash;

/** Fresh, non-deterministic values for things the user creates (tx hashes). */
const live = rng(Date.now() & 0xffffffff);
export const freshHash = (): Hash => randomHash(live);

export const pick = <T>(next: () => number, items: readonly T[]): T => items[Math.floor(next() * items.length)];

export const sleep = (ms: number, signal?: AbortSignal) =>
  new Promise<void>((resolve, reject) => {
    const id = setTimeout(resolve, ms);
    signal?.addEventListener('abort', () => {
      clearTimeout(id);
      reject(new DOMException('Aborted', 'AbortError'));
    });
  });

/** Network-ish latency between `min` and `max` ms. */
export const latency = (min = 280, max = 720, signal?: AbortSignal) =>
  sleep(min + Math.random() * (max - min), signal);

export const nowSec = () => Date.now() / 1000;
