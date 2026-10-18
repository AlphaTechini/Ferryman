<script lang="ts" module>
  import type { Address, Exit, ExitStage, Hash } from '../../services/types';

  export type ActiveExit = {
    receiver: Address;
    txHash: Hash;
    amount: bigint;
    claimTip: bigint;
    recipient: Address;
    startedAt: number;
  };

  export const OUT_STOPS = [0.66, 0.85];

  /** Ferry position: drifts across during maturity, then hops through claim and delivery. */
  export function outProgress(exit: Exit | null, now: number, startedAt: number): number {
    const stage: ExitStage = exit?.stage ?? 'intent';
    if (stage === 'delivered') return 1;
    if (stage === 'claimed') return 0.85;
    if (stage === 'confirmed') return 0.66;
    const end = exit?.maturesAt ?? startedAt + 1500;
    const t = Math.min(1, Math.max(0, (now - startedAt) / Math.max(1, end - startedAt)));
    return 0.06 + t * 0.58;
  }
</script>

<script lang="ts">
  import { explorerAddress, explorerTx } from '../../chains';
  import { clock } from '../../clock.svelte';
  import { formatCountdown, formatDateTime, formatHype, formatInt, shortAddress } from '../../format';
  import { wallet } from '../../services';
  import { ELYSIUM_CHAIN_ID, HYPEREVM_CHAIN_ID } from '../../services/types';
  import { app } from '../../app.svelte';
  import { errorMessage, toasts } from '../../toasts.svelte';
  import Timeline, { type Step } from '../river/Timeline.svelte';
  import Badge from '../ui/Badge.svelte';
  import Button from '../ui/Button.svelte';
  import ExternalLink from '../ui/ExternalLink.svelte';
  import Icon from '../ui/Icon.svelte';

  let { active, exit, onreset }: { active: ActiveExit; exit: Exit | null; onreset: () => void } = $props();

  const stage = $derived<ExitStage>(exit?.stage ?? 'intent');
  const maturesAt = $derived(exit?.maturesAt ?? active.startedAt + 1500);
  const remaining = $derived(Math.max(0, maturesAt - clock.now));
  const payout = $derived(active.amount - active.claimTip);
  const order: ExitStage[] = ['intent', 'confirmed', 'claimed', 'delivered'];
  const idx = $derived(order.indexOf(stage));
  const stepState = (i: number): Step['state'] => (i <= idx ? 'done' : i === idx + 1 ? 'active' : 'pending');

  const steps = $derived<Step[]>([
    {
      id: 'intent',
      title: 'Exit started on Elysium',
      detail: `ExitDesk sent ${formatHype(active.amount)} HYPE to the receiver${exit?.messageId ? ` · message #${formatInt(exit.messageId)}` : ''}.`,
      state: 'done',
      links: [{ label: 'Elysium tx', href: explorerTx(ELYSIUM_CHAIN_ID, active.txHash) }],
    },
    {
      id: 'confirmed',
      title: 'Withdrawal confirmed',
      detail:
        idx >= 1
          ? 'Matured on HyperEVM and ready to claim.'
          : remaining > 0
            ? `Claimable in ${formatCountdown(remaining)}. Elysium posts to HyperEVM in batches; you can close this tab.`
            : 'Due any moment: waiting for the next batch to post.',
      state: stepState(1),
    },
    {
      id: 'claimed',
      title: 'Claimed by the keeper',
      detail: 'claimAndSweep executes the Outbox claim and pays out in one HyperEVM transaction.',
      state: stepState(2),
      links: exit?.claimTx ? [{ label: 'HyperEVM tx', href: explorerTx(HYPEREVM_CHAIN_ID, exit.claimTx) }] : [],
    },
    {
      id: 'delivered',
      title: 'Delivered on HyperEVM',
      detail:
        stage === 'delivered'
          ? `${formatHype(payout)} HYPE paid to ${shortAddress(active.recipient)}${exit?.deliveredAt ? ` · ${formatDateTime(exit.deliveredAt)}` : ''}.`
          : `${formatHype(payout)} HYPE to ${shortAddress(active.recipient)}, after the ${formatHype(active.claimTip)} HYPE keeper tip.`,
      state: stepState(3),
    },
  ]);

  /* Optional landing on HyperCore. Keep a little HYPE back for the transfer's own gas. */
  const CORE_GAS_RESERVE = 1_000_000_000_000_000n; // 0.001 HYPE
  const canLand = $derived(stage === 'delivered' && active.recipient === wallet.account && payout > CORE_GAS_RESERVE);
  let landing = $state<'idle' | 'sending' | 'sent'>('idle');
  let landTx = $state<Hash | null>(null);

  async function land() {
    landing = 'sending';
    try {
      if (wallet.chainId !== HYPEREVM_CHAIN_ID) await wallet.switchChain(HYPEREVM_CHAIN_ID);
      landTx = await wallet.sendToHyperCore(payout - CORE_GAS_RESERVE);
      landing = 'sent';
      toasts.push({ tone: 'success', title: 'Sent to your Hyperliquid balance', body: 'HyperCore credits your spot balance within a few seconds.' });
      app.refreshBalances();
    } catch (err) {
      landing = 'idle';
      toasts.push({ tone: 'error', title: 'Transfer not sent', body: errorMessage(err) });
    }
  }
</script>

<section class="card tracker" aria-labelledby="out-tracker-title">
  <header>
    <div>
      <h2 id="out-tracker-title">{formatHype(active.amount)} HYPE to HyperEVM</h2>
      <p class="faint">Receiver <span class="mono">{shortAddress(active.receiver)}</span></p>
    </div>
    {#if stage === 'delivered'}
      <Badge tone="success"><Icon name="check" size={12} />Delivered</Badge>
    {:else}
      <Badge tone="river"><span class="live" aria-hidden="true"></span>{stage === 'intent' ? 'Maturing' : 'Claiming'}</Badge>
    {/if}
  </header>

  {#if stage === 'intent'}
    <div class="countdown" role="timer" aria-live="off">
      <span class="eyebrow">Claimable in</span>
      <span class="clock num">{formatCountdown(remaining)}</span>
      <span class="faint">Then the keeper claims it for you. Nothing to sign on HyperEVM.</span>
    </div>
  {/if}

  <div class="steps" aria-live="polite">
    <Timeline {steps} label="Ferry Out progress" />
  </div>

  {#if canLand}
    <div class="land">
      {#if landing === 'sent'}
        <p class="landed"><Icon name="check" size={16} />Sent to your Hyperliquid balance.</p>
        {#if landTx}<ExternalLink href={explorerTx(HYPEREVM_CHAIN_ID, landTx)}>Transfer on HyperEVM</ExternalLink>{/if}
      {:else}
        <div class="land-copy">
          <p class="land-title"><Icon name="layers" size={16} />Trade it on Hyperliquid?</p>
          <p class="faint">One HyperEVM signature moves this HYPE to your HyperCore spot balance via Hyperliquid's system address.</p>
        </div>
        <Button variant="primary" loading={landing === 'sending'} onclick={land}>
          {wallet.chainId === HYPEREVM_CHAIN_ID ? 'Send to my Hyperliquid balance' : 'Switch & send to Hyperliquid'}
        </Button>
      {/if}
    </div>
  {/if}

  <footer>
    <ExternalLink href={explorerAddress(HYPEREVM_CHAIN_ID, active.receiver)}>Receiver on HyperEVM</ExternalLink>
    {#if stage === 'delivered'}<Button variant="ghost" size="sm" onclick={onreset}>New exit</Button>{/if}
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

  h2 {
    font-size: var(--fs-lg);
    font-variation-settings: 'opsz' 36;
    letter-spacing: -0.01em;
  }

  header p {
    margin-top: 4px;
    font-size: var(--fs-xs);
  }

  .live {
    width: 6px;
    height: 6px;
    border-radius: var(--r-full);
    background: currentColor;
    animation: blink 1.6s var(--ease-in-out) infinite;
  }

  .countdown {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: var(--s-5) 0;
    border-bottom: 1px solid var(--line);
    font-size: var(--fs-sm);
  }

  .clock {
    font-family: var(--font-display);
    font-size: 3.25rem;
    font-variation-settings: 'opsz' 72;
    letter-spacing: -0.02em;
    line-height: 1.1;
  }

  .steps {
    padding: var(--s-6) 0;
  }

  .land {
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
    padding: var(--s-4);
    margin-bottom: var(--s-5);
    border-radius: var(--r-lg);
    background: var(--bg-sunken);
  }

  .land-title,
  .landed {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    font-weight: 500;
  }

  .landed :global(.icon) {
    color: var(--success);
  }

  .land-copy .faint {
    margin-top: 2px;
    font-size: var(--fs-sm);
  }

  footer {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--s-2);
    padding-top: var(--s-4);
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

    .clock {
      font-size: 2.75rem;
    }
  }
</style>
