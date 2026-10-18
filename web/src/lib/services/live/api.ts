import type {
  Address,
  AppConfig,
  Exit,
  FerryInStatus,
  FerryService,
  Hash,
  Quote,
  RescueClaimResult,
  RescueStats,
  StuckWithdrawal,
  Token,
} from '../types';

const TIMEOUT_MS = 15_000;
const ZERO_ADDRESS = '0x0000000000000000000000000000000000000000' as Address;

/** Fallbacks used for any field the keeper omits. Contract addresses unknown to the app stay zero. */
const DEFAULT_CONFIG: AppConfig = {
  elysium: {
    chainId: 99801,
    rpc: 'https://testnet-rpc.elysium.kinetiq.xyz',
    explorer: 'https://elysium.kinetiq.xyz/testnet-explorer',
  },
  hyperevm: {
    chainId: 998,
    rpc: 'https://rpc.hyperliquid-testnet.xyz/evm',
    explorer: 'https://testnet.purrsec.com',
  },
  contracts: {
    ferryIn: ZERO_ADDRESS,
    exitDesk: ZERO_ADDRESS,
    exitFactory: ZERO_ADDRESS,
    feeJar: ZERO_ADDRESS,
    inbox: '0x11178Df40CBf87a4343f4B3F8B09B366ceFA8Ec0',
    outbox: '0x87391D602aaffB3Fe8051EcCDb75d6EEe8C31060',
  },
  feeBps: 30,
  defaultSlippageBps: 150,
  defaultClaimTip: '500000000000000', // 0.0005 HYPE
};

/** Keeper config over the fallbacks. Null or missing fields fall back; nested objects merge field by field. */
export function mergeConfig(remote: Partial<AppConfig> | null | undefined): AppConfig {
  const r: Partial<AppConfig> = remote ?? {};
  return {
    elysium: { ...DEFAULT_CONFIG.elysium, ...definedOnly(r.elysium) },
    hyperevm: { ...DEFAULT_CONFIG.hyperevm, ...definedOnly(r.hyperevm) },
    contracts: { ...DEFAULT_CONFIG.contracts, ...definedOnly(r.contracts) },
    feeBps: r.feeBps ?? DEFAULT_CONFIG.feeBps,
    defaultSlippageBps: r.defaultSlippageBps ?? DEFAULT_CONFIG.defaultSlippageBps,
    defaultClaimTip: r.defaultClaimTip ?? DEFAULT_CONFIG.defaultClaimTip,
  };
}

/** Drops null, undefined and empty-string values so they cannot overwrite a fallback. */
function definedOnly<T extends object>(obj: Partial<T> | null | undefined): Partial<T> {
  const out: Partial<T> = {};
  for (const [k, v] of Object.entries(obj ?? {})) {
    if (v !== null && v !== undefined && v !== '') (out as Record<string, unknown>)[k] = v;
  }
  return out;
}

type RequestInit2 = { method?: 'GET' | 'POST'; body?: unknown; signal?: AbortSignal };

/** Keeper HTTP client for docs/INTERFACES.md section 3. Amounts stay decimal strings. */
export function createLiveFerryService(apiUrl: string): FerryService {
  const base = apiUrl.replace(/\/+$/, '');

  async function request<T>(path: string, init: RequestInit2 = {}): Promise<T> {
    const controller = new AbortController();
    let timedOut = false;
    const timer = setTimeout(() => {
      timedOut = true;
      controller.abort();
    }, TIMEOUT_MS);
    const callerSignal = init.signal;
    const onCallerAbort = () => controller.abort(callerSignal?.reason);
    if (callerSignal) {
      if (callerSignal.aborted) controller.abort(callerSignal.reason);
      else callerSignal.addEventListener('abort', onCallerAbort, { once: true });
    }

    try {
      const headers: Record<string, string> = { accept: 'application/json' };
      let body: string | undefined;
      if (init.body !== undefined) {
        headers['content-type'] = 'application/json';
        body = JSON.stringify(init.body);
      }

      let res: Response;
      try {
        res = await fetch(`${base}${path}`, {
          method: init.method ?? 'GET',
          headers,
          body,
          signal: controller.signal,
        });
      } catch (err) {
        if (timedOut) throw new Error(`Keeper did not respond within ${TIMEOUT_MS / 1000}s`);
        if (callerSignal?.aborted || (err instanceof Error && err.name === 'AbortError')) throw err;
          throw new Error(`Cannot reach the keeper at ${base}`);
      }

      if (!res.ok) {
        if (res.status === 404) throw new Error('not found');
        const message = await errorMessage(res);
        throw new Error(message);
      }
      return (await res.json()) as T;
    } finally {
      clearTimeout(timer);
      callerSignal?.removeEventListener('abort', onCallerAbort);
    }
  }

  return {
    mode: 'live',

    async getConfig() {
      return mergeConfig(await request<Partial<AppConfig>>('/api/config'));
    },

    getTokens() {
      return request<Token[]>('/api/tokens');
    },

    getQuote(token, amountInWei, slippageBps, signal) {
      const qs = new URLSearchParams({
        token,
        amountIn: amountInWei.toString(),
        slippageBps: String(slippageBps),
      });
      return request<Quote>(`/api/quote?${qs}`, { signal });
    },

    getFerryInStatus(hyperevmTxHash: Hash) {
      return request<FerryInStatus>(`/api/ferry-in/${encodeURIComponent(hyperevmTxHash)}`);
    },

    getExits(address: Address) {
      return request<Exit[]>(`/api/exits?${new URLSearchParams({ address })}`);
    },

    getExit(receiver: Address) {
      return request<Exit>(`/api/exits/${encodeURIComponent(receiver)}`);
    },

    getRescueStats() {
      return request<RescueStats>('/api/rescue/stats');
    },

    lookupStuck(address: Address) {
      return request<StuckWithdrawal[]>(`/api/rescue/lookup?${new URLSearchParams({ address })}`);
    },

    claimForMe(messageId: number) {
      return request<RescueClaimResult>('/api/rescue/claim', { method: 'POST', body: { messageId } });
    },
  };
}

/** Reads `{ "error": "..." }` from a failed response, falling back to the status line. */
async function errorMessage(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { error?: unknown };
    if (typeof body?.error === 'string' && body.error) return body.error;
  } catch {
    /* not JSON */
  }
  return `Keeper request failed (HTTP ${res.status})`;
}
