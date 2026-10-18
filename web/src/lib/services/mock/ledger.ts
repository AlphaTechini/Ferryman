import { encodeAbiParameters, getAddress, keccak256, parseEther } from 'viem';
import type { Address, Exit, ExitParams, FerryInStatus, Hash, RecentRescue, RescueClaimResult, StuckWithdrawal, Token } from '../types';
import { CONFIG, DEMO_ACCOUNT, DEMO_TIMING } from './data';
import { freshHash, hashString, randomAddress, randomHash, rng } from './random';

/**
 * In-memory "chain" shared by the mock wallet (which writes) and the mock keeper API (which reads).
 * Every status is derived from elapsed time, so the UI animates through real stage changes.
 */

type TicketRecord = {
  hyperevmTx: Hash;
  createdAt: number;
  token: Token;
  recipient: Address;
  hypeIn: bigint;
  amountOut: bigint;
  refund: boolean;
  messageId: number;
  elysiumTx: Hash;
};

type ExitRecord = Omit<Exit, 'stage' | 'claimTx' | 'deliveredAt'> & { claimTx: Hash; createdAt: number };

const tickets = new Map<Hash, TicketRecord>();
const exits: ExitRecord[] = [];
let messageCounter = 573_400;

/* ---------- Ferry In ---------- */

export function recordTicket(input: Omit<TicketRecord, 'hyperevmTx' | 'createdAt' | 'messageId' | 'elysiumTx'>): Hash {
  const hyperevmTx = freshHash();
  tickets.set(hyperevmTx, { ...input, hyperevmTx, createdAt: Date.now() / 1000, messageId: ++messageCounter, elysiumTx: freshHash() });
  return hyperevmTx;
}

export function ticketStatus(hash: Hash): FerryInStatus | null {
  const t = tickets.get(hash);
  if (!t) return null;
  const age = Date.now() / 1000 - t.createdAt;
  const stage: FerryInStatus['stage'] =
    age < DEMO_TIMING.relayed ? 'submitted'
    : age < DEMO_TIMING.redeemed ? 'relayed'
    : age < DEMO_TIMING.settled ? 'redeemed'
    : t.refund ? 'refunded'
    : 'bought';
  const settled = stage === 'bought' || stage === 'refunded';
  return {
    hyperevmTx: t.hyperevmTx,
    stage,
    messageId: stage === 'submitted' ? null : t.messageId,
    elysiumTx: stage === 'submitted' || stage === 'relayed' ? null : t.elysiumTx,
    token: t.token.address,
    recipient: t.recipient,
    hypeIn: t.hypeIn.toString(),
    amountOut: settled && !t.refund ? t.amountOut.toString() : null,
    updatedAt: Math.round(t.createdAt + Math.min(age, DEMO_TIMING.settled)),
  };
}

/* ---------- Ferry Out ---------- */

export function exitParams(sender: Address, recipient: Address, amount: bigint, claimTip: bigint, toCore: boolean): ExitParams {
  const nonce = exits.filter((e) => e.params.sender === sender).length;
  return {
    chainId: 99801,
    desk: CONFIG.contracts.exitDesk,
    sender,
    nonce: String(nonce),
    amount: amount.toString(),
    claimTip: claimTip.toString(),
    recipient,
    toCore,
  };
}

const PARAMS_ABI = [
  { type: 'uint256' }, { type: 'address' }, { type: 'address' }, { type: 'uint256' },
  { type: 'uint256' }, { type: 'uint256' }, { type: 'address' }, { type: 'bool' },
] as const;

/** Mirrors the CREATE2 derivation shape (salt = keccak(abi.encode(params))) closely enough for a demo. */
export function exitSalt(p: ExitParams): Hash {
  return keccak256(
    encodeAbiParameters(PARAMS_ABI, [
      BigInt(p.chainId), p.desk, p.sender, BigInt(p.nonce), BigInt(p.amount), BigInt(p.claimTip), p.recipient, p.toCore,
    ]),
  );
}

export function receiverFor(p: ExitParams): Address {
  const digest = keccak256(`0xff${CONFIG.contracts.exitFactory.slice(2)}${exitSalt(p).slice(2)}` as Hash);
  return getAddress(`0x${digest.slice(-40)}`);
}

export function recordExit(params: ExitParams, startedAt = Date.now() / 1000): ExitRecord {
  const record: ExitRecord = {
    receiver: receiverFor(params),
    salt: exitSalt(params),
    params,
    elysiumTx: freshHash(),
    messageId: ++messageCounter,
    startedAt: Math.round(startedAt),
    maturesAt: Math.round(startedAt + DEMO_TIMING.maturity),
    claimTx: freshHash(),
    createdAt: startedAt,
  };
  exits.unshift(record);
  return record;
}

function toExit(r: ExitRecord): Exit {
  const age = Date.now() / 1000 - r.createdAt;
  const stage: Exit['stage'] =
    age < DEMO_TIMING.maturity ? 'intent'
    : age < DEMO_TIMING.claimed ? 'confirmed'
    : age < DEMO_TIMING.delivered ? 'claimed'
    : 'delivered';
  const { createdAt: _createdAt, claimTx, ...rest } = r;
  return {
    ...rest,
    stage,
    claimTx: stage === 'claimed' || stage === 'delivered' ? claimTx : null,
    deliveredAt: stage === 'delivered' ? Math.round(r.createdAt + DEMO_TIMING.delivered) : null,
  };
}

export const exitsFor = (address: Address) =>
  exits.filter((e) => e.params.sender === address || e.params.recipient === address).map(toExit);

export function exitByReceiver(receiver: Address): Exit | null {
  const r = exits.find((e) => e.receiver === receiver);
  return r ? toExit(r) : null;
}

/* Past crossings for the demo account, so "Your crossings" has history. */
{
  const day = 86400;
  const now = Date.now() / 1000;
  const past: [number, string, boolean][] = [
    [6.2 * day, '2.5', false],
    [2.9 * day, '0.84', true],
    [0.4 * day, '1.2075', false],
  ];
  for (const [ago, amount, toCore] of past) {
    recordExit(exitParams(DEMO_ACCOUNT, DEMO_ACCOUNT, parseEther(amount), BigInt(CONFIG.defaultClaimTip), toCore), now - ago);
  }
}

/* ---------- Rescue ---------- */

const rescueRng = rng(0x5717);
const DUST = parseEther('0.002');

/** Typical stranded withdrawal: mostly quest dust, sometimes a real amount. */
function rescueAmount(next: () => number): bigint {
  const r = next();
  if (r < 0.78) return DUST + BigInt(Math.floor(next() * 4)) * 10n ** 14n;
  if (r < 0.95) return parseEther((0.01 + next() * 0.09).toFixed(4));
  return parseEther((0.2 + next() * 1.6).toFixed(3));
}

const rescue = {
  withdrawals: 1182,
  owners: 947,
  hypeReturned: parseEther('3.8741'),
  gasSpent: parseEther('0.41236'),
  lastRunAt: Date.now() / 1000 - 94,
  nextAt: Date.now() / 1000 + 4,
  recent: [] as RecentRescue[],
};

for (let i = 0; i < 8; i++) {
  rescue.recent.push({
    messageId: 573_390 - i * 7 - Math.floor(rescueRng() * 6),
    owner: randomAddress(rescueRng),
    amount: rescueAmount(rescueRng).toString(),
    claimTx: randomHash(rescueRng),
    claimedAt: Math.round(Date.now() / 1000 - 140 - i * (300 + rescueRng() * 900)),
  });
}

/** Advance the keeper's Rescue loop to "now", producing a claim every 5-11 s. */
export function rescueSnapshot() {
  const now = Date.now() / 1000;
  while (rescue.nextAt <= now) {
    const amount = rescueAmount(rescueRng);
    rescue.withdrawals += 1;
    if (rescueRng() < 0.8) rescue.owners += 1;
    rescue.hypeReturned += amount;
    rescue.gasSpent += parseEther('0.00031');
    rescue.lastRunAt = rescue.nextAt;
    rescue.recent.unshift({
      messageId: ++messageCounter,
      owner: randomAddress(rescueRng),
      amount: amount.toString(),
      claimTx: randomHash(rescueRng),
      claimedAt: Math.round(rescue.nextAt),
    });
    rescue.recent.length = Math.min(rescue.recent.length, 12);
    rescue.nextAt += 5 + rescueRng() * 6;
  }
  return rescue;
}

/* Stuck withdrawals per address (deterministic), and claims queued from the UI. */
const queued = new Map<number, { at: number; txHash: Hash }>();
const known = new Map<number, 'sent' | 'confirmed'>();

export function stuckFor(owner: Address): StuckWithdrawal[] {
  const seed = hashString(owner.toLowerCase());
  const next = rng(seed);
  const count = seed % 5 === 0 ? 0 : 1 + (seed % 4);
  const now = Date.now() / 1000;
  return Array.from({ length: count }, (_, i) => {
    const maturing = i === count - 1 && count > 2;
    const messageId = 410_000 + (seed % 150_000) + i * 37;
    known.set(messageId, maturing ? 'sent' : 'confirmed');
    const claim = queued.get(messageId);
    const relayed = claim !== undefined && now - claim.at >= DEMO_TIMING.rescue;
    const status = relayed ? 'relayed' : maturing ? 'sent' : 'confirmed';
    return {
      messageId,
      owner,
      amount: rescueAmount(next).toString(),
      originTx: randomHash(next),
      status,
      ageSeconds: maturing ? Math.round(300 + next() * 900) : Math.round(86400 * (1.2 + next() * 38)),
      claimable: status === 'confirmed',
    };
  });
}

/** Mirrors POST /api/rescue/claim: idempotent, queued first, claimed once the keeper's tx lands. */
export function claimRescue(messageId: number): RescueClaimResult {
  const claim = queued.get(messageId);
  if (claim) {
    const done = Date.now() / 1000 - claim.at >= DEMO_TIMING.rescue;
    return done ? { status: 'claimed', txHash: claim.txHash } : { status: 'queued', txHash: null };
  }
  if (known.get(messageId) !== 'confirmed') return { status: 'not_claimable', txHash: null };
  queued.set(messageId, { at: Date.now() / 1000, txHash: freshHash() });
  return { status: 'queued', txHash: null };
}
