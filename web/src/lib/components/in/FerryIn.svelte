<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { parseEther } from 'viem';
  import { app } from '../../app.svelte';
  import { formatAmount, formatHype, parseAmount, shortAddress, toInputValue } from '../../format';
  import { ferry, wallet } from '../../services';
  import { HYPEREVM_CHAIN_ID, type FerryInStatus, type Quote, type Token } from '../../services/types';
  import { errorMessage, toasts } from '../../toasts.svelte';
  import Crossing from '../river/Crossing.svelte';
  import FlowLayout from '../shell/FlowLayout.svelte';
  import AmountField from '../ui/AmountField.svelte';
  import AssetPill from '../ui/AssetPill.svelte';
  import Button from '../ui/Button.svelte';
  import CopyButton from '../ui/CopyButton.svelte';
  import Icon from '../ui/Icon.svelte';
  import Notes from '../ui/Notes.svelte';
  import Notice from '../ui/Notice.svelte';
  import TokenAvatar from '../ui/TokenAvatar.svelte';
  import InTracker, { IN_PROGRESS, IN_STOPS, type ActiveFerryIn } from './InTracker.svelte';
  import QuoteDetails from './QuoteDetails.svelte';
  import TokenPicker from './TokenPicker.svelte';

  /** HYPE kept back by "Max" so the ticket's gas can still be paid. */
  const GAS_RESERVE = parseEther('0.01');

  let tokens = $state<Token[] | null>(null);
  let tokensError = $state<string | null>(null);
  let token = $state<Token | null>(null);
  let pickerOpen = $state(false);

  let amount = $state('');
  let slippageBps = $state(150);
  let quote = $state<Quote | null>(null);
  let quoting = $state(false);
  let quoteError = $state<string | null>(null);
  let submitting = $state(false);

  let active = $state<ActiveFerryIn | null>(null);
  let status = $state<FerryInStatus | null>(null);

  const amountWei = $derived(parseAmount(amount));
  const balance = $derived(app.balances[HYPEREVM_CHAIN_ID]);
  const feeBps = $derived(app.config?.feeBps ?? 30);

  async function loadTokens() {
    tokensError = null;
    tokens = null;
    try {
      tokens = await ferry.getTokens();
      token ??= tokens.find((t) => t.ourLaunch) ?? tokens[0] ?? null;
    } catch (err) {
      tokensError = errorMessage(err);
    }
  }

  onMount(loadTokens);

  $effect(() => {
    if (app.config) slippageBps = app.config.defaultSlippageBps;
  });

  // Debounced, cancellable quote whenever token, amount or slippage change.
  $effect(() => {
    const t = token;
    const wei = amountWei;
    const bps = slippageBps;
    quoteError = null;
    if (!t || !wei || wei === 0n) {
      quote = null;
      quoting = false;
      return;
    }
    untrack(() => {
      if (quote && quote.token !== t.address) quote = null;
    });
    const ctrl = new AbortController();
    quoting = true;
    const timer = setTimeout(async () => {
      try {
        quote = await ferry.getQuote(t.address, wei, bps, ctrl.signal);
      } catch (err) {
        if (!ctrl.signal.aborted) {
          quote = null;
          quoteError = errorMessage(err);
        }
      } finally {
        if (!ctrl.signal.aborted) quoting = false;
      }
    }, 320);
    return () => {
      clearTimeout(timer);
      ctrl.abort();
    };
  });

  // Follow the ticket until it settles.
  $effect(() => {
    if (!active) return;
    const hash = active.hash;
    let stopped = false;
    let timer: ReturnType<typeof setTimeout>;
    const tick = async () => {
      try {
        const next = await ferry.getFerryInStatus(hash);
        if (stopped) return;
        const settledNow = status?.stage !== next.stage && (next.stage === 'bought' || next.stage === 'refunded');
        status = next;
        if (settledNow && active) announce(next, active);
        if (next.stage === 'bought' || next.stage === 'refunded') return;
      } catch {
        /* not indexed yet; keep polling */
      }
      if (!stopped) timer = setTimeout(tick, 1500);
    };
    tick();
    return () => {
      stopped = true;
      clearTimeout(timer);
    };
  });

  function announce(s: FerryInStatus, a: ActiveFerryIn) {
    if (s.stage === 'bought' && s.amountOut) {
      toasts.push({ tone: 'success', title: `${a.token.symbol} delivered`, body: `${formatAmount(s.amountOut, a.token.decimals)} ${a.token.symbol} are in your wallet on Elysium.` });
    } else if (s.stage === 'refunded') {
      toasts.push({ tone: 'info', title: 'Refunded on Elysium', body: `The price moved past your minimum. ${formatHype(a.amountIn)} HYPE is back in your wallet on Elysium.` });
    }
    app.refreshBalances();
  }

  type Cta = { label: string; action?: () => void; disabled?: boolean; loading?: boolean };
  const cta: Cta = $derived.by(() => {
    if (wallet.status === 'connecting') return { label: 'Connecting', loading: true };
    if (wallet.status !== 'connected') return { label: 'Connect wallet', action: () => (app.walletDialogOpen = true) };
    if (wallet.chainId !== HYPEREVM_CHAIN_ID) return { label: 'Switch to HyperEVM', action: switchChain };
    if (!token) return { label: 'Choose a token', action: () => (pickerOpen = true) };
    if (!amountWei || amountWei === 0n) return { label: 'Enter an amount', disabled: true };
    if (balance !== null && amountWei > balance) return { label: 'Not enough HYPE', disabled: true };
    if (submitting) return { label: 'Confirm in your wallet', loading: true };
    if (quoteError) return { label: 'Quote unavailable', disabled: true };
    if (!quote || quoting) return { label: 'Getting the best price', loading: true };
    if (balance !== null && BigInt(quote.retryable.totalValue) > balance) return { label: 'Not enough HYPE for gas', disabled: true };
    return { label: `Ferry in ${token.symbol}`, action: submit };
  });

  async function switchChain() {
    try {
      await wallet.switchChain(HYPEREVM_CHAIN_ID);
    } catch (err) {
      toasts.push({ tone: 'error', title: "Couldn't switch network", body: errorMessage(err) });
    }
  }

  async function submit() {
    if (!token || !quote || !amountWei) return;
    submitting = true;
    const t = token;
    const q = quote;
    try {
      const hash = await wallet.ferryIn({ token: t, amountWei, minOut: BigInt(q.minOut), quote: q });
      status = null;
      active = { hash, token: t, amountIn: amountWei, expectedOut: BigInt(q.amountOut), startedAt: Date.now() / 1000 };
      toasts.push({ tone: 'info', title: 'Ticket submitted', body: 'Your HYPE is crossing to Elysium. About a minute.' });
      app.refreshBalances();
    } catch (err) {
      toasts.push({ tone: 'error', title: 'Ferry In not sent', body: errorMessage(err) });
    } finally {
      submitting = false;
    }
  }

  function reset() {
    active = null;
    status = null;
    amount = '';
  }

  function max() {
    if (balance === null) return;
    amount = toInputValue(balance > GAS_RESERVE ? balance - GAS_RESERVE : 0n, 18, 4);
  }

  const stage = $derived(status?.stage ?? 'submitted');
  const crossing = $derived.by(() => {
    if (!active) return { progress: 0, status: 'idle' as const, label: 'The ferry is docked on HyperEVM, ready to cross to Elysium.' };
    const settled = stage === 'bought' || stage === 'refunded';
    return {
      progress: IN_PROGRESS[stage],
      status: stage === 'refunded' || stage === 'redeem_failed' ? ('warn' as const) : settled ? ('arrived' as const) : ('moving' as const),
      label: `Ferry In crossing: ${stage.replace('_', ' ')}.`,
    };
  });

  const notes = [
    { term: 'One signature', text: 'You sign once on HyperEVM. No bridge step, no network switch, no hunting for the token on Elysium.' },
    { term: '~60 seconds', text: 'Your HYPE and the buy order travel together as a retryable ticket and are redeemed automatically.' },
    { term: 'Never stuck', text: 'If the price moves past your minimum or the pool is paused, the HYPE is refunded to you on Elysium.' },
    { term: 'Real tokens only', text: 'The list is built from AscendLaunchpad launch events. Look-alikes can copy a name, never an address.' },
  ];
</script>

<FlowLayout
  title="Buy into Elysium without leaving HyperEVM."
  lede="Pick any token launched on Ascend, pay in HYPE from HyperEVM, and it arrives in your wallet on Elysium a minute later."
>
  {#snippet eyebrow()}Ferry In · HyperEVM <Icon name="arrow-right" size={12} /> Elysium{/snippet}

  {#snippet river()}
    <Crossing direction="in" progress={crossing.progress} status={crossing.status} stops={IN_STOPS} label={crossing.label} />
  {/snippet}

  {#snippet main()}
    {#if active}
      <InTracker {active} {status} onreset={reset} />
    {:else}
      <form class="card form" onsubmit={(e) => (e.preventDefault(), cta.action?.())}>
        <AmountField id="ferry-in-amount" label="You pay" bind:value={amount} {balance} onmax={max}>
          {#snippet side()}<AssetPill symbol="HYPE" chain="HyperEVM" />{/snippet}
        </AmountField>

        <div class="joint" aria-hidden="true"><Icon name="arrow-right" size={16} /></div>

        <div class="receive">
          <div class="receive-top">
            <span>You receive</span>
            <span class="faint">on Elysium</span>
          </div>
          <div class="receive-row">
            <output class="out num" class:muted={!quote} for="ferry-in-amount" aria-live="polite">
              {quote && token ? formatAmount(quote.amountOut, token.decimals) : '0'}
            </output>
            <button type="button" class="token" class:empty={!token} onclick={() => (pickerOpen = true)} aria-haspopup="dialog">
              {#if token}
                <TokenAvatar symbol={token.symbol} address={token.address} size={30} />
                <span class="token-symbol">{token.symbol}</span>
              {:else if !tokens && !tokensError}
                <span class="token-symbol">Loading</span>
              {:else}
                <span class="token-symbol">Choose token</span>
              {/if}
              <Icon name="chevron-down" size={16} />
            </button>
          </div>
          {#if token}
            <p class="verified">
              <Icon name="shield" size={13} />
              <span>Verified launch · <span class="mono">{shortAddress(token.address, 6, 4)}</span></span>
              <CopyButton value={token.address} label="Copy token address" />
            </p>
          {/if}
        </div>

        {#if tokensError}
          <Notice tone="danger">Couldn't load tokens: {tokensError} <button type="button" class="link" onclick={loadTokens}>Retry</button></Notice>
        {/if}

        {#if token && amountWei && amountWei > 0n}
          <div class="quote">
            {#if quoteError}
              <Notice tone="danger">{quoteError}</Notice>
            {:else}
              <QuoteDetails {quote} {token} loading={quoting} {feeBps} bind:slippageBps />
            {/if}
          </div>
        {/if}

        <Button type="submit" variant="primary" size="lg" full disabled={cta.disabled} loading={cta.loading}>
          {cta.label}
        </Button>
      </form>
    {/if}
  {/snippet}

  {#snippet aside()}
    <Notes title="The crossing" items={notes} />
  {/snippet}
</FlowLayout>

<TokenPicker
  bind:open={pickerOpen}
  {tokens}
  error={tokensError}
  selected={token}
  onretry={loadTokens}
  onselect={(t) => {
    token = t;
    pickerOpen = false;
  }}
/>

<style>
  .form {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
    padding: var(--s-3);
  }

  .joint {
    position: relative;
    z-index: 1;
    display: grid;
    place-items: center;
    width: 36px;
    height: 36px;
    margin: -22px auto;
    border-radius: var(--r-md);
    background: var(--bg-raised);
    border: 3px solid var(--bg-raised);
    box-shadow: inset 0 0 0 1px var(--line-2);
    color: var(--ink-2);
    transform: rotate(90deg);
  }

  .receive {
    padding: var(--s-4) var(--s-5);
    border-radius: var(--r-lg);
    border: 1px solid var(--line);
  }

  .receive-top {
    display: flex;
    justify-content: space-between;
    font-size: var(--fs-sm);
    color: var(--ink-2);
  }

  .receive-row {
    display: flex;
    align-items: center;
    gap: var(--s-3);
    margin-top: var(--s-2);
  }

  .out {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-display);
    font-size: 2.5rem;
    font-variation-settings: 'opsz' 60;
    letter-spacing: -0.02em;
    line-height: 1.15;
  }

  .out.muted {
    color: var(--ink-3);
    opacity: 0.6;
  }

  .token {
    display: inline-flex;
    align-items: center;
    gap: var(--s-2);
    flex: none;
    height: 44px;
    padding: 0 var(--s-3) 0 6px;
    border-radius: var(--r-full);
    background: var(--bg-sunken);
    border: 1px solid var(--line);
    font-weight: 600;
    font-size: var(--fs-sm);
    transition: border-color var(--dur) var(--ease-out), background-color var(--dur) var(--ease-out);
  }

  .token:hover {
    border-color: var(--line-strong);
  }

  .token.empty {
    padding-left: var(--s-4);
    background: var(--bg-inverse);
    color: var(--ink-inverse);
  }

  .token :global(.icon) {
    color: var(--ink-3);
  }

  .token:focus-visible {
    border-radius: var(--r-full);
  }

  .verified {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-top: var(--s-2);
    font-size: var(--fs-xs);
    color: var(--ink-3);
  }

  .verified :global(.icon) {
    color: var(--success);
  }

  .quote {
    padding: var(--s-4) var(--s-2) var(--s-2);
  }

  .link {
    text-decoration: underline;
    color: inherit;
  }

  .form :global(.btn.lg) {
    margin-top: var(--s-2);
  }

  @media (max-width: 560px) {
    .form {
      padding: var(--s-2);
    }

    .receive {
      padding: var(--s-3) var(--s-4);
    }

    .out {
      font-size: 2rem;
    }
  }
</style>
