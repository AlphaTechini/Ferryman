import { defineChain } from 'viem';
import { ELYSIUM_CHAIN_ID, HYPEREVM_CHAIN_ID, type ChainId } from './services/types';

/** HyperEVM testnet. The RPC needs the `/evm` suffix. */
export const hyperEvmTestnet = defineChain({
  id: HYPEREVM_CHAIN_ID,
  name: 'HyperEVM Testnet',
  nativeCurrency: { name: 'HYPE', symbol: 'HYPE', decimals: 18 },
  rpcUrls: { default: { http: ['https://rpc.hyperliquid-testnet.xyz/evm'] } },
  blockExplorers: { default: { name: 'Purrsec', url: 'https://testnet.purrsec.com' } },
  testnet: true,
});

/** Elysium testnet (99801). Not viem's built-in `elysiumTestnet`, which is chain 1338. */
export const elysiumTestnet = defineChain({
  id: ELYSIUM_CHAIN_ID,
  name: 'Elysium Testnet',
  nativeCurrency: { name: 'HYPE', symbol: 'HYPE', decimals: 18 },
  rpcUrls: { default: { http: ['https://testnet-rpc.elysium.kinetiq.xyz'] } },
  blockExplorers: { default: { name: 'Elysium Explorer', url: 'https://elysium.kinetiq.xyz/testnet-explorer' } },
  testnet: true,
});

export const CHAINS = { [HYPEREVM_CHAIN_ID]: hyperEvmTestnet, [ELYSIUM_CHAIN_ID]: elysiumTestnet } as const;

export const CHAIN_LABEL: Record<ChainId, string> = {
  [HYPEREVM_CHAIN_ID]: 'HyperEVM',
  [ELYSIUM_CHAIN_ID]: 'Elysium',
};

/** Hyperliquid's HYPE system address: transfers here credit the sender's HyperCore spot balance. */
export const HYPERCORE_SYSTEM_ADDRESS = '0x2222222222222222222222222222222222222222';

export function isSupportedChain(id: number | null): id is ChainId {
  return id === HYPEREVM_CHAIN_ID || id === ELYSIUM_CHAIN_ID;
}

export function explorerTx(chainId: ChainId, hash: string): string {
  return `${CHAINS[chainId].blockExplorers.default.url}/tx/${hash}`;
}

export function explorerAddress(chainId: ChainId, address: string): string {
  return `${CHAINS[chainId].blockExplorers.default.url}/address/${address}`;
}
