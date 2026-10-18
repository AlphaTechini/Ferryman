<script lang="ts">
  import { formatEther } from 'viem';
  import { clock } from '../../clock.svelte';
  import { formatHype, formatInt, relativeTime } from '../../format';
  import type { RescueStats } from '../../services/types';
  import Notice from '../ui/Notice.svelte';
  import Skeleton from '../ui/Skeleton.svelte';
  import Ticker from '../ui/Ticker.svelte';

  let { stats, error }: { stats: RescueStats | null; error: string | null } = $props();

  const hype = $derived(stats ? Number(formatEther(BigInt(stats.hypeReturned))) : 0);
  const fmtHype = (n: number) => n.toLocaleString('en-US', { minimumFractionDigits: 4, maximumFractionDigits: 4 });
</script>

<section class="hero" aria-labelledby="rescue-title">
  <p class="eyebrow status">
    <span class="dot" class:off={!stats} aria-hidden="true"></span>
    {#if stats}
      Rescue · keeper last swept {relativeTime(stats.lastRunAt, clock.now)}
    {:else if error}
      Rescue · stats unavailable
    {:else}
      Rescue · connecting to keeper
    {/if}
  </p>

  <h1 id="rescue-title">
    {#if stats}
      <span class="n"><Ticker value={stats.withdrawals} format={formatInt} /></span> stranded withdrawals returned to
      <span class="n"><Ticker value={stats.owners} format={formatInt} /></span> owners.
    {:else}
      <span class="visually-hidden">Loading rescue counter</span>
      <span class="sk" aria-hidden="true"><Skeleton width="min(640px, 90%)" height="0.8em" /><Skeleton width="min(420px, 60%)" height="0.8em" /></span>
    {/if}
  </h1>

  <div class="sub">
    {#if stats}
      <p>
        <span class="amount"><Ticker value={hype} format={fmtHype} /> HYPE</span> delivered to the wallets it belonged to.
        Free for owners; {formatHype(stats.gasSpent, { max: 3 })} HYPE of gas paid by the treasury so far.
      </p>
    {:else if error}
      <p>The keeper's counter couldn't be reached: {error}</p>
    {/if}
    <Notice>
      Most rescued volume is small testnet amounts, often quest dust around 0.002 HYPE. We count every one because each is a
      real withdrawal its owner never came back for, not because it proves demand.
    </Notice>
  </div>
</section>

<style>
  .hero {
    display: flex;
    flex-direction: column;
    gap: var(--s-6);
  }

  .status {
    display: inline-flex;
    align-items: center;
    gap: var(--s-2);
  }

  .dot {
    width: 7px;
    height: 7px;
    border-radius: var(--r-full);
    background: var(--success);
    box-shadow: 0 0 0 3px var(--success-soft);
    animation: breathe 2.4s var(--ease-in-out) infinite;
  }

  .dot.off {
    background: var(--ink-3);
    box-shadow: none;
    animation: none;
  }

  h1 {
    max-width: 19ch;
    font-size: var(--fs-3xl);
    line-height: 1.04;
    color: var(--ink-2);
  }

  .n {
    color: var(--ink);
    font-variant-numeric: lining-nums;
  }

  .sk {
    display: flex;
    flex-direction: column;
    gap: 0.25em;
    padding: 0.1em 0;
  }

  .sub {
    display: grid;
    grid-template-columns: minmax(0, 1.2fr) minmax(0, 1fr);
    gap: var(--s-6) var(--s-12);
    align-items: start;
    padding-top: var(--s-6);
    border-top: 1px solid var(--line-2);
  }

  .sub p {
    font-size: var(--fs-md);
    color: var(--ink-2);
    line-height: 1.5;
  }

  .amount {
    font-family: var(--font-display);
    font-size: 1.5em;
    font-variation-settings: 'opsz' 48;
    letter-spacing: -0.015em;
    color: var(--ink);
    white-space: nowrap;
  }

  @keyframes breathe {
    50% {
      box-shadow: 0 0 0 6px transparent;
    }
  }

  @media (max-width: 860px) {
    .sub {
      grid-template-columns: 1fr;
    }
  }
</style>
