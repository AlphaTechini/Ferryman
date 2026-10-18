import { getAddress, parseEther } from 'viem';
import type { Address, AppConfig, Hash, Token } from '../types';
import { randomAddress, randomHash, rng } from './random';

const next = rng(0xfe44);

export const LAUNCHPAD = getAddress('0x6d01f6644bbf1b5074472D3e85aa1465B4d2bF3d');
const POOL_MANAGER = randomAddress(next);
const USDC = getAddress('0x7D29d8047B905000459c0E80c34a26CeedcB47b2');
const MOCK_USDC = getAddress('0x84Cc00D13173f7016Bbab22A736dBf75A154cFE2');

/** The demo wallets the mock offers in its connect list. */
export const DEMO_ACCOUNT = getAddress('0x7a3f52c1e0b9d84a6f27c3e81b5d9a40c6e2f19d');
export const DEMO_EMPTY_ACCOUNT = getAddress('0x3c90de17a5b4f0628e1d93c7b2a05f8e64d1a7b3');
export const FERRYMAN_LAUNCHER = getAddress('0xf3a1c0de5e7f2b9a4c8d6e1f0a2b3c4d5e6f7a8b');

export const DEMO_TIMING = {
  /** Ferry In stage offsets in seconds (real: ~60 s total). */
  relayed: 5,
  redeemed: 11,
  settled: 16,
  /** Ferry Out maturity in seconds (real: ~25 min), then keeper claim + delivery. */
  maturity: 30,
  claimed: 36,
  delivered: 40,
  /** Rescue claim lands this many seconds after "Claim for me". */
  rescue: 7,
};

export const CONFIG: AppConfig = {
  elysium: { chainId: 99801, rpc: 'https://testnet-rpc.elysium.kinetiq.xyz', explorer: 'https://elysium.kinetiq.xyz/testnet-explorer' },
  hyperevm: { chainId: 998, rpc: 'https://rpc.hyperliquid-testnet.xyz/evm', explorer: 'https://testnet.purrsec.com' },
  contracts: {
    ferryIn: randomAddress(next),
    exitDesk: randomAddress(next),
    exitFactory: randomAddress(next),
    feeJar: randomAddress(next),
    inbox: getAddress('0x11178Df40CBf87a4343f4B3F8B09B366ceFA8Ec0'),
    outbox: getAddress('0x87391D602aaffB3Fe8051EcCDb75d6EEe8C31060'),
  },
  feeBps: 30,
  defaultSlippageBps: 150,
  defaultClaimTip: parseEther('0.0005').toString(),
};

type Seed = {
  name: string;
  symbol: string;
  price: string;
  /** HYPE of liquidity depth, drives price impact. */
  depth: number;
  daysAgo: number;
  quote: 'USDC' | 'mUSDC';
  ours?: boolean;
  launcher?: Address;
};

const SEEDS: Seed[] = [
  { name: 'Ferryman', symbol: 'FERRY', price: '0.000412', depth: 420, daysAgo: 0.6, quote: 'USDC', ours: true, launcher: FERRYMAN_LAUNCHER },
  { name: 'Hypurr Gold', symbol: 'HGOLD', price: '0.0182', depth: 900, daysAgo: 1.4, quote: 'USDC' },
  { name: 'Liquid Monk', symbol: 'MONK', price: '0.00231', depth: 260, daysAgo: 2.1, quote: 'mUSDC' },
  { name: 'Asphodel', symbol: 'ASPH', price: '0.0647', depth: 1500, daysAgo: 3.3, quote: 'USDC' },
  { name: 'Kinetic Cat', symbol: 'KCAT', price: '0.000087', depth: 180, daysAgo: 4.0, quote: 'mUSDC' },
  { name: 'Lantern', symbol: 'LNTRN', price: '0.00519', depth: 340, daysAgo: 5.7, quote: 'USDC' },
  { name: 'Liquid Monk', symbol: 'MONK', price: '0.0000104', depth: 12, daysAgo: 6.2, quote: 'mUSDC' },
  { name: 'Orbit Basis', symbol: 'ORBT', price: '0.112', depth: 2200, daysAgo: 8.9, quote: 'USDC' },
  { name: 'Nectar', symbol: 'NCTR', price: '0.00308', depth: 300, daysAgo: 11.5, quote: 'USDC' },
  { name: 'Otterly', symbol: 'OTTR', price: '0.000641', depth: 95, daysAgo: 14.2, quote: 'mUSDC' },
  { name: 'Kitsune Labs', symbol: 'KITS', price: '0.0249', depth: 700, daysAgo: 19.8, quote: 'USDC' },
  { name: 'Obol', symbol: 'OBOL', price: '0.00095', depth: 210, daysAgo: 26.4, quote: 'USDC' },
];

const created = Date.now() / 1000;

export const TOKENS: Token[] = SEEDS.map((s, i) => {
  const address = randomAddress(next);
  const quoteCurrency = s.quote === 'USDC' ? USDC : MOCK_USDC;
  const [currency0, currency1] = [address, quoteCurrency].sort((a, b) => a.toLowerCase().localeCompare(b.toLowerCase()));
  return {
    address,
    name: s.name,
    symbol: s.symbol,
    decimals: 18,
    launcher: s.launcher ?? randomAddress(next),
    launchTx: randomHash(next) as Hash,
    launchBlock: 4_812_000 - Math.round(s.daysAgo * 86400) + i,
    launchedAt: Math.round(created - s.daysAgo * 86400),
    quoteCurrency,
    quoteSymbol: s.quote,
    poolKey: { currency0, currency1, hooks: LAUNCHPAD, poolManager: POOL_MANAGER, fee: 10000, parameters: '0x00000000000000000000000000000000000000000000000000000000000a0000' },
    priceHype: s.price,
    ourLaunch: Boolean(s.ours),
  };
});

/** Liquidity depth per token address, used by the quote simulation. */
export const DEPTH = new Map(TOKENS.map((t, i) => [t.address, SEEDS[i].depth]));
