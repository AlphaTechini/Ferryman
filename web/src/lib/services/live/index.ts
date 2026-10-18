import type { Services } from '../types';
import { createMockServices } from '../mock';

/**
 * TODO(live): replace with the real implementations:
 *  - FerryService over the keeper HTTP API at `apiUrl` (docs/INTERFACES.md §3), with `mode = 'live'`.
 *  - WalletService over viem + EIP-6963 discovery (window.ethereum fallback), `$state`-backed.
 * Until then this falls back to the demo services, which still report `mode = 'mock'`,
 * so the UI keeps showing its "Demo data" label.
 */
export function createLiveServices(_apiUrl: string): Services {
  return createMockServices();
}
