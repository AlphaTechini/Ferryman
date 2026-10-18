/**
 * The boundary between the Ferryman UI and everything chain- or keeper-related.
 *
 * - API shapes mirror docs/INTERFACES.md §3 exactly (amounts are decimal wei strings,
 *   addresses checksummed, times unix seconds).
 * - `FerryService` wraps the keeper HTTP API.
 * - `WalletService` wraps the user's wallet (EIP-6963 discovery, viem clients, signing).
 *
 * Implementations live in ./mock (default, demo data) and ./live (keeper + viem).
 * Reactive members of WalletService (account, chainId, status, providers) MUST be backed by
 * Svelte 5 `$state` (e.g. class fields in a `.svelte.ts` module) so components update.
 */

export type Address = `0x${string}`;
export type Hash = `0x${string}`;

export const HYPEREVM_CHAIN_ID = 998;
export const ELYSIUM_CHAIN_ID = 99801;
export type ChainId = typeof HYPEREVM_CHAIN_ID | typeof ELYSIUM_CHAIN_ID;

/* ------------------------------------------------------------------ */
/* Keeper API types (INTERFACES.md §3)                                 */
/* ------------------------------------------------------------------ */

export type PoolKey = {
  currency0: string;
  currency1: string;
  hooks: string;
  poolManager: string;
  fee: number;
  parameters: string;
};

export type Token = {
  address: Address;
  name: string;
  symbol: string;
  decimals: number;
  launcher: Address;
  launchTx: Hash;
  launchBlock: number;
  launchedAt: number;
  quoteCurrency: Address;
  quoteSymbol: string;
  poolKey: PoolKey;
  /** Token price in HYPE, decimal string, display only. */
  priceHype: string | null;
  ourLaunch: boolean;
};

export type Quote = {
  token: Address;
  amountIn: string;
  fee: string;
  amountOut: string;
  minOut: string;
  slippageBps: number;
  priceImpactBps: number;
  route: string[];
  simulatedAtBlock: number;
  retryable: {
    gasLimit: string;
    maxFeePerGas: string;
    maxSubmissionCost: string;
    totalValue: string;
  };
};

export type FerryInStage = 'submitted' | 'relayed' | 'redeemed' | 'bought' | 'refunded' | 'redeem_failed';

export type FerryInStatus = {
  hyperevmTx: Hash;
  stage: FerryInStage;
  messageId: number | null;
  elysiumTx: Hash | null;
  token: Address | null;
  recipient: Address;
  hypeIn: string;
  amountOut: string | null;
  updatedAt: number;
};

export type ExitParams = {
  chainId: number;
  desk: Address;
  sender: Address;
  nonce: string;
  amount: string;
  claimTip: string;
  recipient: Address;
  toCore: boolean;
};

export type ExitStage = 'intent' | 'confirmed' | 'claimed' | 'delivered';

export type Exit = {
  receiver: Address;
  salt: Hash;
  params: ExitParams;
  stage: ExitStage;
  elysiumTx: Hash;
  messageId: number | null;
  startedAt: number;
  maturesAt: number | null;
  claimTx: Hash | null;
  deliveredAt: number | null;
};

export type RecentRescue = {
  messageId: number;
  owner: Address;
  amount: string;
  claimTx: Hash;
  claimedAt: number;
};

export type RescueStats = {
  withdrawals: number;
  owners: number;
  hypeReturned: string;
  gasSpent: string;
  lastRunAt: number;
  recent: RecentRescue[];
};

export type StuckWithdrawal = {
  messageId: number;
  owner: Address;
  amount: string;
  originTx: Hash;
  status: 'initiated' | 'sent' | 'confirmed' | 'relayed';
  ageSeconds: number;
  claimable: boolean;
};

export type RescueClaimResult = {
  status: 'queued' | 'claimed' | 'not_claimable';
  txHash: Hash | null;
};

export type ChainConfig = { chainId: number; rpc: string; explorer: string };

export type AppConfig = {
  elysium: ChainConfig;
  hyperevm: ChainConfig;
  contracts: {
    ferryIn: Address;
    exitDesk: Address;
    exitFactory: Address;
    feeJar: Address;
    inbox: Address;
    outbox: Address;
  };
  feeBps: number;
  defaultSlippageBps: number;
  defaultClaimTip: string;
};

/* ------------------------------------------------------------------ */
/* Services                                                            */
/* ------------------------------------------------------------------ */

export interface FerryService {
  /** 'mock' makes the UI show the "Demo data" label. Live must return 'live'. */
  readonly mode: 'mock' | 'live';
  getConfig(): Promise<AppConfig>;
  /** Verified list built from AscendLaunchpad `launch` events only. */
  getTokens(): Promise<Token[]>;
  getQuote(token: Address, amountInWei: bigint, slippageBps: number, signal?: AbortSignal): Promise<Quote>;
  getFerryInStatus(hyperevmTxHash: Hash): Promise<FerryInStatus>;
  /** Exits where `address` is the sender or the recipient, newest first. */
  getExits(address: Address): Promise<Exit[]>;
  getExit(receiver: Address): Promise<Exit>;
  getRescueStats(): Promise<RescueStats>;
  lookupStuck(address: Address): Promise<StuckWithdrawal[]>;
  claimForMe(messageId: number): Promise<RescueClaimResult>;
}

export type WalletStatus = 'disconnected' | 'connecting' | 'connected';

/** One wallet the user can pick: an EIP-6963 announcement, or the window.ethereum fallback. */
export type WalletProviderInfo = {
  /** EIP-6963 `uuid`, or 'injected' for the window.ethereum fallback. */
  id: string;
  name: string;
  /** Data URI from the EIP-6963 announcement (may be empty for the injected fallback). */
  icon: string;
  rdns: string;
};

export type FerryInArgs = {
  token: Token;
  amountWei: bigint;
  minOut: bigint;
  /** The quote the user accepted; carries retryable gas params and totalValue. */
  quote: Quote;
};

export type FerryOutArgs = {
  recipient: Address;
  amountWei: bigint;
  claimTip: bigint;
  toCore: boolean;
};

export type ReceiverPreview = { params: ExitParams; receiver: Address };

export interface WalletService {
  readonly status: WalletStatus;
  readonly account: Address | null;
  readonly chainId: number | null;
  /** Wallets discovered via EIP-6963, plus an 'injected' fallback when only window.ethereum exists. */
  readonly providers: WalletProviderInfo[];

  /** Start (or restart) EIP-6963 discovery. Safe to call repeatedly. */
  discover(): void;
  /** Connect to a provider from `providers` (defaults to the first one). Resolves to the account. */
  connect(providerId?: string): Promise<Address>;
  disconnect(): Promise<void>;
  switchChain(chainId: ChainId): Promise<void>;
  /** wallet_addEthereumChain for Elysium testnet (99801). */
  addElysiumNetwork(): Promise<void>;
  /** Native HYPE balance of the connected account on the given chain, in wei. */
  getBalance(chainId: ChainId): Promise<bigint>;

  /** HyperEVM: Inbox.createRetryableTicket -> FerryIn.buy(token, account, minOut). Returns the HyperEVM tx hash. */
  ferryIn(args: FerryInArgs): Promise<Hash>;
  /** Elysium: ExitDesk.previewReceiver(account, recipient, amount, claimTip, toCore). Read-only. */
  previewReceiver(args: FerryOutArgs): Promise<ReceiverPreview>;
  /** Elysium: ExitDesk.exitHYPE{value: amountWei}(recipient, claimTip, toCore). */
  ferryOut(args: FerryOutArgs): Promise<{ txHash: Hash; receiver: Address }>;
  /** HyperEVM: plain HYPE transfer to 0x2222…2222, credited to the sender's HyperCore spot balance. */
  sendToHyperCore(amountWei: bigint): Promise<Hash>;
}

export type Services = { ferry: FerryService; wallet: WalletService };

/** Errors services throw for expected user-facing outcomes. */
export class UserRejectedError extends Error {
  constructor(message = 'Request rejected in wallet') {
    super(message);
    this.name = 'UserRejectedError';
  }
}
