import {
  createPublicClient,
  createWalletClient,
  custom,
  encodeFunctionData,
  getAddress,
  http,
  parseEventLogs,
  size,
  toHex,
  type EIP1193Provider,
  type PublicClient,
  type WalletClient,
} from 'viem';
import { CHAINS, elysiumTestnet, HYPERCORE_SYSTEM_ADDRESS, hyperEvmTestnet } from '../../chains';
import {
  ELYSIUM_CHAIN_ID,
  HYPEREVM_CHAIN_ID,
  type Address,
  type AppConfig,
  type ChainId,
  type FerryInArgs,
  type FerryOutArgs,
  type Hash,
  type ReceiverPreview,
  type WalletProviderInfo,
  type WalletService,
  type WalletStatus,
} from '../types';
import { exitDeskAbi, ferryInAbi, inboxAbi } from './abis';
import { guard, isUnknownChain } from './errors';
import { toExitParams } from './exit';
import { needsSubmissionFee, resolveRetryable } from './retryable';

const PROVIDER_KEY = 'ferryman.wallet.provider';
const ZERO_ADDRESS = '0x0000000000000000000000000000000000000000';
const INJECTED_ID = 'injected';

/** EIP-1193 provider with the event methods the wallet subscribes to. */
type Provider = EIP1193Provider & {
  on(event: string, listener: (...args: any[]) => void): void;
  removeListener(event: string, listener: (...args: any[]) => void): void;
};

type Entry = { info: WalletProviderInfo; provider: Provider };

type AnnounceDetail = { info: { uuid: string; name: string; icon: string; rdns: string }; provider: Provider };

type Eip6963Window = { ethereum?: Provider };

/**
 * Wallet over EIP-6963 (with a window.ethereum fallback) and viem.
 * Reactive fields are `$state` so the UI updates on connect, account and network changes.
 */
export class LiveWallet implements WalletService {
  status: WalletStatus = $state('disconnected');
  account: Address | null = $state(null);
  chainId: number | null = $state(null);
  providers: WalletProviderInfo[] = $state([]);

  readonly #getConfig: () => Promise<AppConfig>;
  readonly #publicClients: Record<ChainId, PublicClient>;
  /** EIP-6963 announcements, deduplicated by rdns. */
  #announced: Entry[] = [];
  #entry: Entry | null = null;
  #walletClient: WalletClient | null = null;

  constructor(getConfig: () => Promise<AppConfig>) {
    this.#getConfig = getConfig;
    this.#publicClients = {
      [HYPEREVM_CHAIN_ID]: createPublicClient({ chain: hyperEvmTestnet, transport: http() }),
      [ELYSIUM_CHAIN_ID]: createPublicClient({ chain: elysiumTestnet, transport: http() }),
    };
    if (typeof window !== 'undefined') {
      window.addEventListener('eip6963:announceProvider', this.#onAnnounce);
    }
    this.discover();
    void this.#restore();
  }

  /* ---------------------------------------------------------------- */
  /* Discovery and connection                                          */
  /* ---------------------------------------------------------------- */

  discover(): void {
    if (typeof window === 'undefined') return;
    window.dispatchEvent(new Event('eip6963:requestProvider'));
    this.#refreshProviders();
  }

  async connect(providerId?: string): Promise<Address> {
    this.discover();
    const entry = this.#findEntry(providerId);
    if (!entry) throw new Error('No browser wallet found. Install one and reload the page.');

    this.status = 'connecting';
    try {
      const accounts = await guard(() => entry.provider.request({ method: 'eth_requestAccounts' }));
      return await this.#attach(entry, accounts);
    } catch (err) {
      this.status = this.account ? 'connected' : 'disconnected';
      throw err;
    }
  }

  async disconnect(): Promise<void> {
    this.#reset();
    persistProvider(null);
  }

  async switchChain(chainId: ChainId): Promise<void> {
    const provider = this.#requireProvider();
    await guard(async () => {
      try {
        await provider.request({ method: 'wallet_switchEthereumChain', params: [{ chainId: toHex(chainId) }] });
      } catch (err) {
        if (!isUnknownChain(err)) throw err;
        await provider.request({ method: 'wallet_addEthereumChain', params: [addChainParams(chainId)] });
        await provider.request({ method: 'wallet_switchEthereumChain', params: [{ chainId: toHex(chainId) }] });
      }
    });
    this.chainId = chainId;
  }

  async addElysiumNetwork(): Promise<void> {
    const provider = this.#requireProvider();
    await guard(() =>
      provider.request({ method: 'wallet_addEthereumChain', params: [addChainParams(ELYSIUM_CHAIN_ID)] }),
    );
  }

  async getBalance(chainId: ChainId): Promise<bigint> {
    const account = this.#requireAccount();
    return this.#publicClients[chainId].getBalance({ address: account });
  }

  /* ---------------------------------------------------------------- */
  /* HyperEVM: Ferry In and HyperCore transfer                         */
  /* ---------------------------------------------------------------- */

  async ferryIn({ token, amountWei, minOut, quote }: FerryInArgs): Promise<Hash> {
    const account = this.#requireAccount();
    if (quote.amountIn !== amountWei.toString()) {
      throw new Error('This quote is for a different amount. Refresh the quote and try again.');
    }
    const config = await this.#getConfig();
    const ferryIn = requireDeployed(config.contracts.ferryIn, 'Ferry In is not deployed yet');
    const inbox = requireDeployed(config.contracts.inbox, 'Inbox address is not configured');

    const data = encodeFunctionData({
      abi: ferryInAbi,
      functionName: 'buy',
      args: [token.address, account, minOut],
    });

    const { wallet } = await this.#prepareWrite(HYPEREVM_CHAIN_ID);
    const hyper = this.#publicClients[HYPEREVM_CHAIN_ID];

    const minSubmissionFee = needsSubmissionFee(quote.retryable)
      ? await hyper.readContract({
          address: inbox,
          abi: inboxAbi,
          functionName: 'calculateRetryableSubmissionFee',
          args: [BigInt(size(data)), 0n],
        })
      : undefined;

    const retry = resolveRetryable(quote.retryable, amountWei, minSubmissionFee);
    if (retry.recomputed) {
      console.warn('Keeper retryable values differ from the local calculation; using the recomputed totalValue.');
    }

    const { request } = await hyper.simulateContract({
      account,
      address: inbox,
      abi: inboxAbi,
      functionName: 'createRetryableTicket',
      args: [
        ferryIn,
        amountWei,
        retry.maxSubmissionCost,
        account,
        account,
        retry.gasLimit,
        retry.maxFeePerGas,
        data,
      ],
      value: retry.totalValue,
      chain: CHAINS[HYPEREVM_CHAIN_ID],
    });
    return guard(() => wallet.writeContract(request));
  }

  async sendToHyperCore(amountWei: bigint): Promise<Hash> {
    const { wallet, account } = await this.#prepareWrite(HYPEREVM_CHAIN_ID);
    return guard(() =>
      wallet.sendTransaction({
        account,
        chain: CHAINS[HYPEREVM_CHAIN_ID],
        to: HYPERCORE_SYSTEM_ADDRESS,
        value: amountWei,
      }),
    );
  }

  /* ---------------------------------------------------------------- */
  /* Elysium: Ferry Out                                                */
  /* ---------------------------------------------------------------- */

  async previewReceiver(args: FerryOutArgs): Promise<ReceiverPreview> {
    const account = this.#requireAccount();
    const config = await this.#getConfig();
    const exitDesk = requireDeployed(config.contracts.exitDesk, 'Exit Desk is not deployed yet');
    return this.#preview(exitDesk, account, args);
  }

  async ferryOut(args: FerryOutArgs): Promise<{ txHash: Hash; receiver: Address }> {
    const config = await this.#getConfig();
    const exitDesk = requireDeployed(config.contracts.exitDesk, 'Exit Desk is not deployed yet');
    const { wallet, account } = await this.#prepareWrite(ELYSIUM_CHAIN_ID);
    const elysium = this.#publicClients[ELYSIUM_CHAIN_ID];

    // The receiver depends on the sender's nonce, so preview right before sending.
    const preview = await this.#preview(exitDesk, account, args);

    const { request } = await elysium.simulateContract({
      account,
      address: exitDesk,
      abi: exitDeskAbi,
      functionName: 'exitHYPE',
      args: [args.recipient, args.claimTip, args.toCore],
      value: args.amountWei,
      chain: CHAINS[ELYSIUM_CHAIN_ID],
    });
    const txHash = await guard(() => wallet.writeContract(request));

    const receipt = await elysium.waitForTransactionReceipt({ hash: txHash });
    if (receipt.status !== 'success') throw new Error('The exit transaction reverted on Elysium');

    const [intent] = parseEventLogs({ abi: exitDeskAbi, eventName: 'ExitIntent', logs: receipt.logs }).filter(
      (log) => log.address.toLowerCase() === exitDesk.toLowerCase(),
    );
    if (!intent) throw new Error('ExitIntent event not found in the exit transaction');

    // The event is the truth: it is what the contract actually recorded.
    if (intent.args.receiver.toLowerCase() !== preview.receiver.toLowerCase()) {
      console.warn('Exit receiver differs from the preview; using the receiver from ExitIntent.');
    }
    return { txHash, receiver: intent.args.receiver };
  }

  /* ---------------------------------------------------------------- */
  /* Internals                                                         */
  /* ---------------------------------------------------------------- */

  #onAnnounce = (event: Event) => {
    const detail = (event as CustomEvent<AnnounceDetail>).detail;
    if (!detail?.info?.rdns || !detail.provider) return;
    if (this.#announced.some((e) => e.info.rdns === detail.info.rdns)) return;
    this.#announced.push({
      info: {
        id: detail.info.uuid,
        name: detail.info.name,
        icon: detail.info.icon,
        rdns: detail.info.rdns,
      },
      provider: detail.provider,
    });
    this.#refreshProviders();
  };

  #onAccountsChanged = (accounts: string[]) => {
    if (!accounts.length) {
      this.#reset();
      return;
    }
    this.account = getAddress(accounts[0]);
  };

  #onChainChanged = (chainHex: string) => {
    this.chainId = Number(BigInt(chainHex));
  };

  #onDisconnect = () => {
    this.#reset();
  };

  /** Entries from EIP-6963, or the window.ethereum fallback when none announced. */
  #entries(): Entry[] {
    if (this.#announced.length) return this.#announced;
    if (typeof window === 'undefined') return [];
    const injected = (window as unknown as Eip6963Window).ethereum;
    if (!injected) return [];
    return [{ info: { id: INJECTED_ID, name: 'Browser wallet', icon: '', rdns: '' }, provider: injected }];
  }

  #findEntry(id?: string): Entry | null {
    const entries = this.#entries();
    if (!id) return entries[0] ?? null;
    return entries.find((e) => e.info.id === id) ?? null;
  }

  #refreshProviders() {
    this.providers = this.#entries().map((e) => e.info);
  }

  #requireAccount(): Address {
    if (!this.account) throw new Error('Connect a wallet first');
    return this.account;
  }

  #requireProvider(): Provider {
    if (!this.#entry) throw new Error('Connect a wallet first');
    return this.#entry.provider;
  }

  async #prepareWrite(chainId: ChainId): Promise<{ wallet: WalletClient; account: Address }> {
    const account = this.#requireAccount();
    if (this.chainId !== chainId) await this.switchChain(chainId);
    const wallet = this.#walletClient;
    if (!wallet) throw new Error('Connect a wallet first');
    return { wallet, account };
  }

  async #preview(exitDesk: Address, account: Address, args: FerryOutArgs): Promise<ReceiverPreview> {
    const [params, receiver] = await this.#publicClients[ELYSIUM_CHAIN_ID].readContract({
      address: exitDesk,
      abi: exitDeskAbi,
      functionName: 'previewReceiver',
      args: [account, args.recipient, args.amountWei, args.claimTip, args.toCore],
    });
    return { params: toExitParams(params), receiver };
  }

  /** Makes the given entry the active one: client, listeners, state and persisted choice. */
  async #attach(entry: Entry, accounts: readonly string[]): Promise<Address> {
    if (!accounts.length) throw new Error('The wallet returned no account');
    const account = getAddress(accounts[0]);
    const chainHex = await entry.provider.request({ method: 'eth_chainId' });

    this.#detach();
    this.#entry = entry;
    this.#walletClient = createWalletClient({ transport: custom(entry.provider) });
    entry.provider.on('accountsChanged', this.#onAccountsChanged);
    entry.provider.on('chainChanged', this.#onChainChanged);
    entry.provider.on('disconnect', this.#onDisconnect);

    this.account = account;
    this.chainId = Number(BigInt(chainHex));
    this.status = 'connected';
    persistProvider(entry.info.id);
    return account;
  }

  #detach() {
    const entry = this.#entry;
    if (entry) {
      entry.provider.removeListener('accountsChanged', this.#onAccountsChanged);
      entry.provider.removeListener('chainChanged', this.#onChainChanged);
      entry.provider.removeListener('disconnect', this.#onDisconnect);
    }
    this.#entry = null;
    this.#walletClient = null;
  }

  #reset() {
    this.#detach();
    this.account = null;
    this.chainId = null;
    this.status = 'disconnected';
  }

  /** Silently reconnects to the last provider (eth_accounts, no prompt). */
  async #restore() {
    const id = readPersistedProvider();
    if (!id) return;
    const entry = this.#findEntry(id);
    if (!entry) return;

    this.status = 'connecting';
    try {
      const accounts = await entry.provider.request({ method: 'eth_accounts' });
      if (!this.account && accounts.length) await this.#attach(entry, accounts);
    } catch {
      /* not connected: stay disconnected, the user can connect again */
    } finally {
      if (!this.account && this.status === 'connecting') this.status = 'disconnected';
    }
  }
}

function requireDeployed(address: Address, message: string): Address {
  if (!address || address.toLowerCase() === ZERO_ADDRESS) throw new Error(message);
  return address;
}

/** wallet_addEthereumChain params, taken from the chain definitions in chains.ts. */
function addChainParams(chainId: ChainId) {
  const chain = CHAINS[chainId];
  return {
    chainId: toHex(chain.id),
    chainName: chain.name,
    nativeCurrency: chain.nativeCurrency,
    rpcUrls: [chain.rpcUrls.default.http[0]],
    blockExplorerUrls: [chain.blockExplorers.default.url],
  };
}

function readPersistedProvider(): string | null {
  try {
    return localStorage.getItem(PROVIDER_KEY);
  } catch {
    return null;
  }
}

function persistProvider(id: string | null) {
  try {
    if (id) localStorage.setItem(PROVIDER_KEY, id);
    else localStorage.removeItem(PROVIDER_KEY);
  } catch {
    /* storage unavailable: the wallet just won't auto-reconnect */
  }
}
