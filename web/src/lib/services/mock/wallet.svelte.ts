import { parseEther } from 'viem';
import {
  ELYSIUM_CHAIN_ID,
  HYPEREVM_CHAIN_ID,
  type Address,
  type ChainId,
  type FerryInArgs,
  type FerryOutArgs,
  type Hash,
  type WalletProviderInfo,
  type WalletService,
  type WalletStatus,
} from '../types';
import { DEMO_ACCOUNT, DEMO_EMPTY_ACCOUNT, TOKENS } from './data';
import { exitParams, receiverFor, recordExit, recordTicket } from './ledger';
import { simulateQuote } from './ferry';
import { freshHash, sleep } from './random';

const STORAGE_KEY = 'ferryman.demo-wallet';

function icon(fill: string): string {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="8" fill="${fill}"/><path d="M8 18h16l-3 5H11z" fill="#f3eee4"/><path d="M14 18l4-9" stroke="#f3eee4" stroke-width="1.6" stroke-linecap="round"/></svg>`;
  return `data:image/svg+xml,${encodeURIComponent(svg)}`;
}

const DEMO_PROVIDERS: (WalletProviderInfo & { account: Address; balances: Record<ChainId, bigint> })[] = [
  {
    id: 'demo-funded',
    name: 'Demo wallet',
    rdns: 'xyz.ferryman.demo',
    icon: icon('#1f4a55'),
    account: DEMO_ACCOUNT,
    balances: { [HYPEREVM_CHAIN_ID]: parseEther('12.4821'), [ELYSIUM_CHAIN_ID]: parseEther('3.2507') },
  },
  {
    id: 'demo-empty',
    name: 'Demo wallet, nearly empty',
    rdns: 'xyz.ferryman.demo-empty',
    icon: icon('#7a5a2b'),
    account: DEMO_EMPTY_ACCOUNT,
    balances: { [HYPEREVM_CHAIN_ID]: parseEther('0.0013'), [ELYSIUM_CHAIN_ID]: parseEther('0.0004') },
  },
];

function readStored(): { id: string; chainId: number } | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? JSON.parse(raw) : null;
  } catch {
    return null;
  }
}

function store(value: { id: string; chainId: number } | null) {
  try {
    if (value) localStorage.setItem(STORAGE_KEY, JSON.stringify(value));
    else localStorage.removeItem(STORAGE_KEY);
  } catch {
    /* storage unavailable: the demo wallet just won't auto-reconnect */
  }
}

/** A simulated wallet. Signing waits ~1.4 s to feel like a wallet prompt. */
export class MockWalletService implements WalletService {
  status: WalletStatus = $state('disconnected');
  account: Address | null = $state(null);
  chainId: number | null = $state(null);
  providers: WalletProviderInfo[] = $state([]);

  #active: (typeof DEMO_PROVIDERS)[number] | null = null;

  constructor() {
    this.discover();
    const saved = readStored();
    const provider = DEMO_PROVIDERS.find((p) => p.id === saved?.id);
    if (saved && provider) this.#attach(provider, saved.chainId);
  }

  discover() {
    this.providers = DEMO_PROVIDERS.map(({ id, name, icon, rdns }) => ({ id, name, icon, rdns }));
  }

  #attach(provider: (typeof DEMO_PROVIDERS)[number], chainId: number) {
    this.#active = provider;
    this.account = provider.account;
    this.chainId = chainId;
    this.status = 'connected';
    store({ id: provider.id, chainId });
  }

  #require(chainId?: ChainId) {
    const active = this.#active;
    if (!active || !this.account) throw new Error('Connect a wallet first');
    if (chainId && this.chainId !== chainId) throw new Error('Wrong network');
    return active;
  }

  async connect(providerId?: string) {
    const provider = DEMO_PROVIDERS.find((p) => p.id === providerId) ?? DEMO_PROVIDERS[0];
    this.status = 'connecting';
    await sleep(650);
    this.#attach(provider, HYPEREVM_CHAIN_ID);
    return provider.account;
  }

  async disconnect() {
    this.#active = null;
    this.account = null;
    this.chainId = null;
    this.status = 'disconnected';
    store(null);
  }

  async switchChain(chainId: ChainId) {
    const active = this.#require();
    await sleep(500);
    this.#attach(active, chainId);
  }

  async addElysiumNetwork() {
    await sleep(700);
  }

  async getBalance(chainId: ChainId) {
    await sleep(180);
    return this.#active?.balances[chainId] ?? 0n;
  }

  async ferryIn({ token, amountWei, minOut, quote }: FerryInArgs): Promise<Hash> {
    const active = this.#require(HYPEREVM_CHAIN_ID);
    await sleep(1400);
    const total = BigInt(quote.retryable.totalValue);
    if (total > active.balances[HYPEREVM_CHAIN_ID]) throw new Error('Insufficient HYPE for value plus gas');
    active.balances[HYPEREVM_CHAIN_ID] -= total;
    // The ticket lands ~60 s later at whatever the pool says then; an impossible minOut refunds.
    const settled = BigInt(simulateQuote(token.address, amountWei, 0).amountOut);
    return recordTicket({
      token: TOKENS.find((t) => t.address === token.address) ?? token,
      recipient: active.account,
      hypeIn: amountWei,
      amountOut: settled,
      refund: minOut > settled,
    });
  }

  async previewReceiver({ recipient, amountWei, claimTip, toCore }: FerryOutArgs) {
    const active = this.#require();
    await sleep(240);
    const params = exitParams(active.account, recipient, amountWei, claimTip, toCore);
    return { params, receiver: receiverFor(params) };
  }

  async ferryOut({ recipient, amountWei, claimTip, toCore }: FerryOutArgs) {
    const active = this.#require(ELYSIUM_CHAIN_ID);
    await sleep(1400);
    if (amountWei > active.balances[ELYSIUM_CHAIN_ID]) throw new Error('Insufficient HYPE on Elysium');
    active.balances[ELYSIUM_CHAIN_ID] -= amountWei;
    const exit = recordExit(exitParams(active.account, recipient, amountWei, claimTip, toCore));
    return { txHash: exit.elysiumTx, receiver: exit.receiver };
  }

  async sendToHyperCore(amountWei: bigint) {
    const active = this.#require(HYPEREVM_CHAIN_ID);
    await sleep(1200);
    active.balances[HYPEREVM_CHAIN_ID] -= amountWei;
    return freshHash();
  }
}
