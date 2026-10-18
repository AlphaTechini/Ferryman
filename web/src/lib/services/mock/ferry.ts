import { parseEther } from 'viem';
import type { Address, FerryService, Hash, Quote } from '../types';
import { CONFIG, DEPTH, TOKENS } from './data';
import { claimRescue, exitByReceiver, exitsFor, rescueSnapshot, stuckFor, ticketStatus } from './ledger';
import { latency, nowSec } from './random';

const BPS = 10_000n;
const GAS_LIMIT = 1_250_000n;
const MAX_FEE_PER_GAS = 100_000_000n; // 0.1 gwei
const SUBMISSION_COST = parseEther('0.0000214');

/** Price impact against a pool with `depth` HYPE of liquidity, in bps. */
function impactBps(amountHype: number, depth: number): number {
  return Math.min(4_900, Math.round((amountHype / (amountHype + depth)) * 10_000));
}

/** Simulated Ferry In quote: fee, then HYPE -> quote -> token at the pool price with impact. */
export function simulateQuote(token: Address, amountIn: bigint, slippageBps: number): Quote {
  const t = TOKENS.find((x) => x.address === token);
  if (!t || !t.priceHype) throw new Error('No pool found for this token');
  const fee = (amountIn * BigInt(CONFIG.feeBps)) / BPS;
  const net = amountIn - fee;
  const priceWei = parseEther(t.priceHype);
  const impact = impactBps(Number(net) / 1e18, DEPTH.get(token) ?? 200);
  const amountOut = (((net * 10n ** 18n) / priceWei) * (BPS - BigInt(impact))) / BPS;
  const minOut = (amountOut * (BPS - BigInt(slippageBps))) / BPS;
  const gasCost = GAS_LIMIT * MAX_FEE_PER_GAS;
  return {
    token,
    amountIn: amountIn.toString(),
    fee: fee.toString(),
    amountOut: amountOut.toString(),
    minOut: minOut.toString(),
    slippageBps,
    priceImpactBps: impact,
    route: ['HYPE', t.quoteSymbol, t.symbol],
    simulatedAtBlock: 4_812_000 + Math.floor((nowSec() % 1e6) / 0.25),
    retryable: {
      gasLimit: GAS_LIMIT.toString(),
      maxFeePerGas: MAX_FEE_PER_GAS.toString(),
      maxSubmissionCost: SUBMISSION_COST.toString(),
      totalValue: (amountIn + SUBMISSION_COST + gasCost).toString(),
    },
  };
}

export class MockFerryService implements FerryService {
  readonly mode = 'mock' as const;

  async getConfig() {
    await latency(120, 260);
    return CONFIG;
  }

  async getTokens() {
    await latency(500, 900);
    return TOKENS;
  }

  async getQuote(token: Address, amountInWei: bigint, slippageBps: number, signal?: AbortSignal) {
    await latency(350, 750, signal);
    return simulateQuote(token, amountInWei, slippageBps);
  }

  async getFerryInStatus(hyperevmTxHash: Hash) {
    await latency(120, 300);
    const status = ticketStatus(hyperevmTxHash);
    if (!status) throw new Error('Ticket not found yet');
    return status;
  }

  async getExits(address: Address) {
    await latency(300, 700);
    return exitsFor(address);
  }

  async getExit(receiver: Address) {
    await latency(120, 300);
    const exit = exitByReceiver(receiver);
    if (!exit) throw new Error('Exit not indexed yet');
    return exit;
  }

  async getRescueStats() {
    await latency(200, 500);
    const s = rescueSnapshot();
    return {
      withdrawals: s.withdrawals,
      owners: s.owners,
      hypeReturned: s.hypeReturned.toString(),
      gasSpent: s.gasSpent.toString(),
      lastRunAt: Math.round(s.lastRunAt),
      recent: s.recent.slice(0, 8),
    };
  }

  async lookupStuck(address: Address) {
    await latency(600, 1100);
    return stuckFor(address);
  }

  async claimForMe(messageId: number) {
    await latency(500, 900);
    return claimRescue(messageId);
  }
}
