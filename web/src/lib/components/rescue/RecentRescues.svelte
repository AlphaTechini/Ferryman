<script lang="ts">
  import { fly } from 'svelte/transition';
  import { prefersReducedMotion } from 'svelte/motion';
  import { explorerAddress, explorerTx } from '../../chains';
  import { clock } from '../../clock.svelte';
  import { formatHype, formatInt, relativeTime, shortAddress } from '../../format';
  import { HYPEREVM_CHAIN_ID, type RecentRescue } from '../../services/types';
  import Skeleton from '../ui/Skeleton.svelte';

  let { items }: { items: RecentRescue[] | null } = $props();
</script>

<section class="recent" aria-labelledby="recent-title">
  <div class="head">
    <h2 id="recent-title">Recent rescues</h2>
    <span class="live"><span class="dot" aria-hidden="true"></span>Live</span>
  </div>

  {#if !items}
    <ul aria-busy="true" aria-label="Loading recent rescues">
      {#each Array(6) as _, i (i)}
        <li class="row"><Skeleton width="40%" height="14px" /><Skeleton width="22%" height="12px" /></li>
      {/each}
    </ul>
  {:else if items.length === 0}
    <p class="empty">No rescues yet. The keeper sweeps every few minutes.</p>
  {:else}
    <ul aria-live="polite" aria-relevant="additions">
      {#each items as r (r.messageId)}
        <li class="row" in:fly={{ y: prefersReducedMotion.current ? 0 : -10, duration: 420 }}>
          <div class="what">
            <span class="amount num">{formatHype(r.amount)} <span class="unit">HYPE</span></span>
            <span class="to">
              to <a class="mono" href={explorerAddress(HYPEREVM_CHAIN_ID, r.owner)} target="_blank" rel="noopener noreferrer">{shortAddress(r.owner)}<span class="visually-hidden"> (opens explorer)</span></a>
            </span>
          </div>
          <div class="when">
            <span>{relativeTime(r.claimedAt, clock.now)}</span>
            <a class="mono faint" href={explorerTx(HYPEREVM_CHAIN_ID, r.claimTx)} target="_blank" rel="noopener noreferrer">
              #{formatInt(r.messageId)}<span class="visually-hidden"> claim transaction (opens explorer)</span>
            </a>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</section>

<style>
  .head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    padding-bottom: var(--s-4);
    border-bottom: 1px solid var(--line-2);
  }

  h2 {
    font-size: var(--fs-xl);
  }

  .live {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-family: var(--font-mono);
    font-size: var(--fs-2xs);
    letter-spacing: var(--tracking-caps);
    text-transform: uppercase;
    color: var(--success);
  }

  .dot {
    width: 6px;
    height: 6px;
    border-radius: var(--r-full);
    background: currentColor;
    animation: blink 1.8s var(--ease-in-out) infinite;
  }

  .row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: var(--s-4);
    padding: var(--s-3) 0;
    border-bottom: 1px solid var(--line);
    min-height: 60px;
  }

  .what,
  .when {
    display: flex;
    flex-direction: column;
    min-width: 0;
    font-size: var(--fs-sm);
  }

  .when {
    align-items: flex-end;
    color: var(--ink-2);
    white-space: nowrap;
  }

  .amount {
    font-weight: 500;
  }

  .unit {
    color: var(--ink-3);
    font-weight: 400;
    font-size: var(--fs-xs);
  }

  .to {
    color: var(--ink-3);
    font-size: var(--fs-xs);
  }

  .to a {
    color: var(--ink-2);
    text-decoration: none;
  }

  .when a {
    font-size: var(--fs-xs);
    text-decoration: none;
  }

  a:hover {
    text-decoration: underline;
  }

  .empty {
    padding: var(--s-6) 0;
    color: var(--ink-2);
    font-size: var(--fs-sm);
  }

  @keyframes blink {
    50% {
      opacity: 0.35;
    }
  }
</style>
