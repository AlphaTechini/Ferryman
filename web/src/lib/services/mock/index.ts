import type { Services } from '../types';
import { MockFerryService } from './ferry';
import { MockWalletService } from './wallet.svelte';

/** Demo services: realistic data that advances over time. Used whenever VITE_API_URL is unset. */
export function createMockServices(): Services {
  return { ferry: new MockFerryService(), wallet: new MockWalletService() };
}
