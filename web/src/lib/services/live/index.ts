import type { Services } from '../types';
import { createLiveFerryService } from './api';
import { LiveWallet } from './wallet.svelte';

/** Live services: keeper HTTP API (`ferry`, mode 'live') and the viem-backed wallet. */
export function createLiveServices(apiUrl: string): Services {
  const ferry = createLiveFerryService(apiUrl);
  // The wallet reads contract addresses from the keeper's config.
  const wallet = new LiveWallet(() => ferry.getConfig());
  return { ferry, wallet };
}
