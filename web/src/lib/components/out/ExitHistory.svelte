<script lang="ts">
  import { explorerTx } from '../../chains';
  import { formatDateTime, formatHype, relativeTime, shortAddress } from '../../format';
  import { clock } from '../../clock.svelte';
  import { ELYSIUM_CHAIN_ID, type Address, type Exit, type ExitStage } from '../../services/types';
  import Badge from '../ui/Badge.svelte';
  import Button from '../ui/Button.svelte';
  import Icon from '../ui/Icon.svelte';
  import Skeleton from '../ui/Skeleton.svelte';

  let {
    exits,
    error,
    account,
    current,
    onopen,
    onretry,
  }: {
    exits: Exit[] | null;
    error: string | null;
    account: Address | null;
    current: Address | null;
    onopen: (exit: Exit) => void;
    onretry: () => void;
  } = $props();

  const LABEL: Record<ExitStage, string> = {
    intent: 'Maturing',
    confirmed: 'Ready to claim',
    claimed: 'Claimed',
    delivered: 'Delivered',
  };
</script>

<section class="history" aria-labelledby="history-title">
  <div class="head">
    <h2 id="history-title">Your crossings</h2>
    {#if exits?.length}<span class="faint">{exits.length} {exits.length === 1 ? 'exit' : 'exits'}</span>{/if}
  </div>

  {#if !account}
    <p class="empty">Connect a wallet to see exits you've sent or received.</p>
  {:else if error}
    <div class="empty">
      <p>Couldn't load your crossings. {error}</p>
      <Button size="sm" onclick={onretry}><Icon name="refresh" size={15} />Try again</Button>
    </div>
  {:else if !exits}
    <ul class="list" aria-busy="true" aria-label="Loading crossings">
      {#each Array(3) as _, i (i)}
        <li class="row sk"><Skeleton width="30%" height="14px" /><Skeleton width="18%" height="14px" /></li>
      {/each}
    </ul>
  {:else if exits.length === 0}
    <p class="empty">No crossings yet. Exits sent from or to {shortAddress(account)} will appear here.</p>
  {:else}
    <ul class="list">
      {#each exits as e (e.receiver)}
        {@const payout = BigInt(e.params.amount) - BigInt(e.params.claimTip)}
        <li class="row" class:current={current === e.receiver}>
          <div class="amount">
            <span class="value num">{formatHype(payout)} <span class="unit">HYPE</span></span>
            <span class="faint">to {e.params.recipient === account ? 'you' : shortAddress(e.params.recipient)}</span>
          </div>
          <div class="when">
            <span title={formatDateTime(e.startedAt)}>{relativeTime(e.startedAt, clock.now)}</span>
            <a class="faint mono" href={explorerTx(ELYSIUM_CHAIN_ID, e.elysiumTx)} target="_blank" rel="noopener noreferrer">
              {shortAddress(e.elysiumTx, 4, 4)}<span class="visually-hidden"> (Elysium transaction, opens explorer)</span>
            </a>
          </div>
          <div class="stage">
            <Badge tone={e.stage === 'delivered' ? 'success' : e.stage === 'intent' ? 'river' : 'lantern'}>{LABEL[e.stage]}</Badge>
          </div>
          <button class="open" onclick={() => onopen(e)} aria-label="Track exit of {formatHype(payout)} HYPE">
            <Icon name="arrow-right" size={16} />
          </button>
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
    gap: var(--s-4);
    padding-bottom: var(--s-4);
    border-bottom: 1px solid var(--line-2);
  }

  h2 {
    font-size: var(--fs-xl);
  }

  .head .faint {
    font-size: var(--fs-sm);
  }

  .list {
    display: flex;
    flex-direction: column;
  }

  .row {
    display: grid;
    grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr) auto 40px;
    align-items: center;
    gap: var(--s-4);
    padding: var(--s-4) 0;
    border-bottom: 1px solid var(--line);
  }

  .row.sk {
    grid-template-columns: 1fr auto;
  }

  .row.current .value {
    color: var(--lantern-ink);
  }

  .amount,
  .when {
    display: flex;
    flex-direction: column;
    min-width: 0;
    font-size: var(--fs-sm);
  }

  .value {
    font-family: var(--font-display);
    font-size: var(--fs-lg);
    font-variation-settings: 'opsz' 36;
    letter-spacing: -0.01em;
    line-height: 1.25;
  }

  .unit {
    font-family: var(--font-sans);
    font-size: var(--fs-xs);
    color: var(--ink-3);
    letter-spacing: 0;
  }

  .when a {
    font-size: var(--fs-xs);
    text-decoration: none;
  }

  .when a:hover {
    text-decoration: underline;
  }

  .open {
    display: grid;
    place-items: center;
    width: 36px;
    height: 36px;
    border-radius: var(--r-full);
    color: var(--ink-2);
    border: 1px solid var(--line);
    transition: border-color var(--dur) var(--ease-out), color var(--dur) var(--ease-out);
  }

  .open:hover {
    color: var(--ink);
    border-color: var(--line-strong);
  }

  .open:focus-visible {
    border-radius: var(--r-full);
  }

  .empty {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--s-3);
    padding: var(--s-6) 0;
    color: var(--ink-2);
    font-size: var(--fs-sm);
  }

  @media (max-width: 560px) {
    .row {
      grid-template-columns: minmax(0, 1fr) auto;
      grid-template-areas:
        'amount stage'
        'when open';
      row-gap: var(--s-2);
    }

    .amount {
      grid-area: amount;
    }

    .when {
      grid-area: when;
      flex-direction: row;
      gap: var(--s-2);
    }

    .stage {
      grid-area: stage;
      justify-self: end;
    }

    .open {
      grid-area: open;
      justify-self: end;
      width: 32px;
      height: 32px;
    }
  }
</style>
