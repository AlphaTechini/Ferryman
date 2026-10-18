<script lang="ts" module>
  import type { FerryInStage, Hash, Token } from '../../services/types';

  export type ActiveFerryIn = { hash: Hash; token: Token; amountIn: bigint; expectedOut: bigint; startedAt: number };

  /** Where the ferry sits on the river for each stage. */
  export const IN_PROGRESS: Record<FerryInStage, number> = {
    submitted: 0.14,
    relayed: 0.42,
    redeemed: 0.72,
    redeem_failed: 0.6,
    bought: 1,
    refunded: 1,
  };
  export const IN_STOPS = [0.42, 0.72];
</script>

<script lang="ts">
  import { explorerTx } from '../../chains';
  import { clock } from '../../clock.svelte';
  import { formatAmount, formatCountdown, formatHype, formatInt } from '../../format';
  import { wallet } from '../../services';
  import { ELYSIUM_CHAIN_ID, HYPEREVM_CHAIN_ID, type FerryInStatus } from '../../services/types';
  import { errorMessage, toasts } from '../../toasts.svelte';
  import Timeline, { type Step } from '../river/Timeline.svelte';
  import Badge from '../ui/Badge.svelte';
  import Button from '../ui/Button.svelte';
  import Icon from '../ui/Icon.svelte';
  import TokenAvatar from '../ui/TokenAvatar.svelte';

  let { active, status, onreset }: { active: ActiveFerryIn; status: FerryInStatus | null; onreset: () => void } = $props();

  const stage = $derived<FerryInStage>(status?.stage ?? 'submitted');
  const done = $derived(stage === 'bought' || stage === 'refunded');
  const elapsed = $derived(Math.max(0, (done && status ? status.updatedAt : clock.now) - active.startedAt));
  const order: FerryInStage[] = ['submitted', 'relayed', 'redeemed', 'bought'];
  const reached = (s: FerryInStage) => {
    const at = stage === 'refunded' ? 3 : stage === 'redeem_failed' ? 2 : order.indexOf(stage);
    return order.indexOf(s) <= at;
  };
  const stateOf = (s: FerryInStage): Step['state'] => {
    if (!reached(s)) return 'pending';
    if (s === stage && !done) return 'active';
    return 'done';
  };

  const steps = $derived<Step[]>([
    {
      id: 'submitted',
      title: 'Submitted on HyperEVM',
      detail: 'Retryable ticket created with your HYPE and the buy order.',
      state: stage === 'submitted' ? 'active' : 'done',
      links: [{ label: 'HyperEVM tx', href: explorerTx(HYPEREVM_CHAIN_ID, active.hash) }],
    },
    {
      id: 'relayed',
      title: 'Relayed to Elysium',
      detail: status?.messageId ? `Message #${formatInt(status.messageId)} reached the Elysium inbox.` : 'Waiting for Elysium to pick up the ticket.',
      state: stateOf('relayed'),
    },
    {
      id: 'redeemed',
      title: stage === 'redeem_failed' ? 'Auto-redeem failed, keeper retrying' : 'Redeemed on Elysium',
      detail:
        stage === 'redeem_failed'
          ? 'The ticket is safe. Ferryman’s keeper is redeeming it for you; nothing to do.'
          : `FerryIn swaps HYPE → ${active.token.quoteSymbol} → ${active.token.symbol}.`,
      state: stage === 'redeem_failed' ? 'active' : stateOf('redeemed'),
      links: status?.elysiumTx ? [{ label: 'Elysium tx', href: explorerTx(ELYSIUM_CHAIN_ID, status.elysiumTx) }] : [],
    },
    stage === 'refunded'
      ? {
          id: 'refunded',
          title: 'Refunded on Elysium',
          detail: `The price moved past your minimum, so ${formatHype(active.amountIn)} HYPE went back to you on Elysium. Nothing is stuck.`,
          state: 'failed',
        }
      : {
          id: 'bought',
          title: 'Tokens delivered',
          detail:
            stage === 'bought' && status?.amountOut
              ? `${formatAmount(status.amountOut, active.token.decimals)} ${active.token.symbol} in your wallet on Elysium.`
              : `About ${formatAmount(active.expectedOut, active.token.decimals)} ${active.token.symbol} to your address.`,
          state: stage === 'bought' ? 'done' : 'pending',
        },
  ]);

  let adding = $state(false);
  async function addElysium() {
    adding = true;
    try {
      await wallet.addElysiumNetwork();
      toasts.push({ tone: 'success', title: 'Elysium added to your wallet', body: 'Switch to it any time to see your tokens.' });
    } catch (err) {
      toasts.push({ tone: 'error', title: "Couldn't add Elysium", body: errorMessage(err) });
    } finally {
      adding = false;
    }
  }
</script>

<section class="card tracker" aria-labelledby="in-tracker-title">
  <header>
    <div class="summary">
      <TokenAvatar symbol={active.token.symbol} address={active.token.address} size={40} />
      <div>
        <h2 id="in-tracker-title">
          {formatHype(active.amountIn)} HYPE <Icon name="arrow-right" size={16} /> {active.token.symbol}
        </h2>
        <p class="faint num">
          {#if done}Settled in {formatCountdown(elapsed)}{:else}{formatCountdown(elapsed)} elapsed · usually ~60 s{/if}
        </p>
      </div>
    </div>
    {#if stage === 'bought'}
      <Badge tone="success"><Icon name="check" size={12} />Delivered</Badge>
    {:else if stage === 'refunded'}
      <Badge tone="lantern"><Icon name="undo" size={12} />Refunded</Badge>
    {:else}
      <Badge tone="river"><span class="live" aria-hidden="true"></span>Crossing</Badge>
    {/if}
  </header>

  <div class="steps" aria-live="polite">
    <Timeline {steps} label="Ferry In progress" />
  </div>

  <footer>
    <Button variant="secondary" loading={adding} onclick={addElysium}>
      {#if !adding}<Icon name="plus" size={15} />{/if}Add Elysium to wallet
    </Button>
    {#if done}
      <Button variant="ghost" onclick={onreset}>New crossing</Button>
    {/if}
  </footer>
</section>

<style>
  .tracker {
    padding: var(--s-6);
  }

  header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--s-4);
    padding-bottom: var(--s-5);
    border-bottom: 1px solid var(--line);
  }

  .summary {
    display: flex;
    gap: var(--s-3);
    align-items: center;
    min-width: 0;
  }

  h2 {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    font-size: var(--fs-lg);
    font-variation-settings: 'opsz' 36;
    letter-spacing: -0.01em;
  }

  h2 :global(.icon) {
    color: var(--ink-3);
  }

  .summary p {
    margin-top: 2px;
    font-size: var(--fs-xs);
  }

  .live {
    width: 6px;
    height: 6px;
    border-radius: var(--r-full);
    background: currentColor;
    animation: blink 1.6s var(--ease-in-out) infinite;
  }

  .steps {
    padding: var(--s-6) 0;
  }

  footer {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s-2);
    padding-top: var(--s-5);
    border-top: 1px solid var(--line);
  }

  @keyframes blink {
    50% {
      opacity: 0.3;
    }
  }

  @media (max-width: 560px) {
    .tracker {
      padding: var(--s-5) var(--s-4);
    }
  }
</style>
