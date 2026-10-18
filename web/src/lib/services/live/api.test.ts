import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createLiveFerryService, mergeConfig } from './api';

const BASE = 'http://localhost:8787';
const ADDR = '0x1111111111111111111111111111111111111111' as const;
const HASH = `0x${'ab'.repeat(32)}` as const;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } });
}

describe('live ferry service (keeper HTTP API)', () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn(async () => json([]));
    vi.stubGlobal('fetch', fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  const lastUrl = () => String(fetchMock.mock.calls.at(-1)?.[0]);
  const lastInit = () => fetchMock.mock.calls.at(-1)?.[1] as RequestInit;

  it('has mode live and hits the documented endpoints', async () => {
    const ferry = createLiveFerryService(`${BASE}/`);
    expect(ferry.mode).toBe('live');

    await ferry.getTokens();
    expect(lastUrl()).toBe(`${BASE}/api/tokens`);

    await ferry.getFerryInStatus(HASH);
    expect(lastUrl()).toBe(`${BASE}/api/ferry-in/${HASH}`);

    await ferry.getExit(ADDR);
    expect(lastUrl()).toBe(`${BASE}/api/exits/${ADDR}`);

    await ferry.getRescueStats();
    expect(lastUrl()).toBe(`${BASE}/api/rescue/stats`);
  });

  it('builds query strings for quote, exits and rescue lookup', async () => {
    const ferry = createLiveFerryService(BASE);

    await ferry.getQuote(ADDR, 1_000_000_000_000_000_000n, 150);
    const quote = new URL(lastUrl());
    expect(quote.pathname).toBe('/api/quote');
    expect(quote.searchParams.get('token')).toBe(ADDR);
    expect(quote.searchParams.get('amountIn')).toBe('1000000000000000000');
    expect(quote.searchParams.get('slippageBps')).toBe('150');

    await ferry.getExits(ADDR);
    expect(lastUrl()).toBe(`${BASE}/api/exits?address=${ADDR}`);

    await ferry.lookupStuck(ADDR);
    expect(lastUrl()).toBe(`${BASE}/api/rescue/lookup?address=${ADDR}`);
  });

  it('POSTs the rescue claim as JSON', async () => {
    const ferry = createLiveFerryService(BASE);
    fetchMock.mockResolvedValueOnce(json({ status: 'queued', txHash: null }));

    const result = await ferry.claimForMe(573261);

    expect(result).toEqual({ status: 'queued', txHash: null });
    expect(lastUrl()).toBe(`${BASE}/api/rescue/claim`);
    expect(lastInit().method).toBe('POST');
    expect(lastInit().body).toBe(JSON.stringify({ messageId: 573261 }));
  });

  it('maps 404 to "not found" and JSON error bodies to their message', async () => {
    const ferry = createLiveFerryService(BASE);

    fetchMock.mockResolvedValueOnce(json({ error: 'nope' }, 404));
    await expect(ferry.getExit(ADDR)).rejects.toThrow('not found');

    fetchMock.mockResolvedValueOnce(json({ error: 'quote unavailable' }, 422));
    await expect(ferry.getQuote(ADDR, 1n, 150)).rejects.toThrow('quote unavailable');

    fetchMock.mockResolvedValueOnce(new Response('<html>oops</html>', { status: 502 }));
    await expect(ferry.getTokens()).rejects.toThrow('HTTP 502');
  });

  it('turns network failures into a readable error', async () => {
    const ferry = createLiveFerryService(BASE);
    fetchMock.mockRejectedValueOnce(new TypeError('fetch failed'));
    await expect(ferry.getTokens()).rejects.toThrow(`Cannot reach the keeper at ${BASE}`);
  });

  it('times out after 15 seconds', async () => {
    vi.useFakeTimers();
    const ferry = createLiveFerryService(BASE);
    fetchMock.mockImplementationOnce(
      (_url: string, init: RequestInit) =>
        new Promise((_resolve, reject) => {
          init.signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')));
        }),
    );

    const pending = ferry.getTokens();
    const assertion = expect(pending).rejects.toThrow('Keeper did not respond within 15s');
    await vi.advanceTimersByTimeAsync(15_000);
    await assertion;
  });

  it('passes the caller signal through and rejects with its abort', async () => {
    const ferry = createLiveFerryService(BASE);
    const caller = new AbortController();
    fetchMock.mockImplementationOnce(
      (_url: string, init: RequestInit) =>
        new Promise((_resolve, reject) => {
          init.signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')));
        }),
    );

    const pending = ferry.getQuote(ADDR, 1n, 150, caller.signal);
    caller.abort();
    await expect(pending).rejects.toMatchObject({ name: 'AbortError' });
  });
});

describe('getConfig merge', () => {
  it('uses fallbacks when the keeper omits fields', () => {
    const config = mergeConfig({ feeBps: 25, contracts: { ferryIn: ADDR } as never });
    expect(config.feeBps).toBe(25);
    expect(config.defaultSlippageBps).toBe(150);
    expect(config.contracts.ferryIn).toBe(ADDR);
    expect(config.contracts.exitDesk).toBe('0x0000000000000000000000000000000000000000');
    expect(config.contracts.inbox).toBe('0x11178Df40CBf87a4343f4B3F8B09B366ceFA8Ec0');
    expect(config.elysium.chainId).toBe(99801);
    expect(config.hyperevm.rpc).toBe('https://rpc.hyperliquid-testnet.xyz/evm');
  });

  it('does not let null values overwrite fallbacks', () => {
    const config = mergeConfig({ contracts: { inbox: null, ferryIn: ADDR } as never, feeBps: null as never });
    expect(config.contracts.inbox).toBe('0x11178Df40CBf87a4343f4B3F8B09B366ceFA8Ec0');
    expect(config.feeBps).toBe(30);
  });
});
