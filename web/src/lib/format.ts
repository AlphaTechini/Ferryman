import { formatUnits, parseUnits } from 'viem';

type Amount = bigint | string;

const toBig = (v: Amount): bigint => (typeof v === 'bigint' ? v : BigInt(v));

/** Group the integer part of a plain decimal string: "12345.67" -> "12,345.67". */
function group(decimal: string): string {
  const [int, frac] = decimal.split('.');
  const grouped = int.replace(/\B(?=(\d{3})+(?!\d))/g, ',');
  return frac ? `${grouped}.${frac}` : grouped;
}

/** Truncate (never round up) a decimal string to `places` fraction digits and trim zeros. */
function truncate(decimal: string, places: number): string {
  const [int, frac = ''] = decimal.split('.');
  const cut = frac.slice(0, places).replace(/0+$/, '');
  return cut ? `${int}.${cut}` : int;
}

/**
 * Human amount with precision that scales with size:
 * 1,204.5 · 12.4821 · 0.0021 · <0.0001. Values are truncated, never rounded up.
 */
export function formatAmount(value: Amount, decimals = 18, opts: { max?: number } = {}): string {
  const wei = toBig(value);
  if (wei === 0n) return '0';
  const negative = wei < 0n;
  const abs = negative ? -wei : wei;
  const plain = formatUnits(abs, decimals);
  const whole = Number(plain.split('.')[0]);
  const places = opts.max ?? (whole >= 1000 ? 2 : whole >= 1 ? 4 : 6);
  let out = truncate(plain, places);
  if (out === '0') {
    const floor = `0.${'0'.repeat(Math.max(places - 1, 0))}1`;
    return `<${floor}`;
  }
  out = group(out);
  return negative ? `-${out}` : out;
}

export const formatHype = (wei: Amount, opts?: { max?: number }) => formatAmount(wei, 18, opts);

/** Parse user input ("1.5", "1,000", ".2") into base units. Returns null when not a valid positive-or-zero number. */
export function parseAmount(input: string, decimals = 18): bigint | null {
  const clean = input.replace(/,/g, '').trim();
  if (!clean || !/^\d*\.?\d*$/.test(clean) || clean === '.') return null;
  const [, frac = ''] = clean.split('.');
  if (frac.length > decimals) return null;
  try {
    return parseUnits(clean.startsWith('.') ? `0${clean}` : clean, decimals);
  } catch {
    return null;
  }
}

/** Plain, ungrouped decimal for filling an input (e.g. "Max"). */
export function toInputValue(wei: bigint, decimals = 18, places = 6): string {
  return truncate(formatUnits(wei, decimals), places);
}

/** 0x11178Df40CBf87a4343f4B3F8B09B366ceFA8Ec0 -> 0x1117…8Ec0 */
export function shortAddress(address: string, lead = 4, tail = 4): string {
  if (address.length <= 2 + lead + tail + 1) return address;
  return `${address.slice(0, 2 + lead)}…${address.slice(-tail)}`;
}

export const formatInt = (n: number) => Math.round(n).toLocaleString('en-US');

/** 150 -> "1.5%" */
export function formatBps(bps: number): string {
  return `${(bps / 100).toLocaleString('en-US', { maximumFractionDigits: 2 })}%`;
}

/** Seconds between now and `unix` as "just now", "4 min ago", "in 12 min", "3 d ago". */
export function relativeTime(unix: number, now = Date.now() / 1000): string {
  const delta = Math.round(unix - now);
  const abs = Math.abs(delta);
  if (abs < 10) return 'just now';
  const units: [number, string][] = [
    [86400 * 30, 'mo'],
    [86400, 'd'],
    [3600, 'h'],
    [60, 'min'],
    [1, 's'],
  ];
  const [size, label] = units.find(([s]) => abs >= s) ?? [1, 's'];
  const n = Math.floor(abs / size);
  return delta < 0 ? `${n} ${label} ago` : `in ${n} ${label}`;
}

/** Approximate durations: 60 -> "~60 s", 1440 -> "~24 min", 7200 -> "~2 h". */
export function formatDuration(seconds: number): string {
  if (seconds < 90) return `~${Math.round(seconds)} s`;
  if (seconds < 5400) return `~${Math.round(seconds / 60)} min`;
  if (seconds < 172800) return `~${Math.round(seconds / 3600)} h`;
  return `~${Math.round(seconds / 86400)} d`;
}

/** Age of a withdrawal: 93784 -> "1 d 2 h", 5400 -> "1 h 30 min". */
export function formatAge(seconds: number): string {
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (d > 0) return h ? `${d} d ${h} h` : `${d} d`;
  if (h > 0) return m ? `${h} h ${m} min` : `${h} h`;
  return `${Math.max(m, 1)} min`;
}

/** Countdown clock: 1122 -> "18:42", 3725 -> "1:02:05". */
export function formatCountdown(seconds: number): string {
  const s = Math.max(0, Math.ceil(seconds));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = String(s % 60).padStart(2, '0');
  return h ? `${h}:${String(m).padStart(2, '0')}:${sec}` : `${m}:${sec}`;
}

/** "Oct 18, 14:02" in the user's locale. */
export function formatDateTime(unix: number): string {
  return new Date(unix * 1000).toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}
