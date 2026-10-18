<script lang="ts">
  import { getAddress, isAddress } from 'viem';
  import { app } from '../../app.svelte';
  import { clock } from '../../clock.svelte';
  import { formatDuration, formatHype, parseAmount, shortAddress, toInputValue } from '../../format';
  import { ferry, wallet } from '../../services';
  import { ELYSIUM_CHAIN_ID, type Address, type Exit, type ReceiverPreview as Preview } from '../../services/types';
  import { errorMessage, toasts } from '../../toasts.svelte';
  import Crossing from '../river/Crossing.svelte';
  import FlowLayout from '../shell/FlowLayout.svelte';
  import AmountField from '../ui/AmountField.svelte';
  import AssetPill from '../ui/AssetPill.svelte';
  import Button from '../ui/Button.svelte';
  import Icon from '../ui/Icon.svelte';
  import Notes from '../ui/Notes.svelte';
  import Tooltip from '../ui/Tooltip.svelte';
  import ExitHistory from './ExitHistory.svelte';
  import OutTracker, { OUT_STOPS, outProgress, type ActiveExit } from './OutTracker.svelte';
  import ReceiverPreview from './ReceiverPreview.svelte';

  const GAS_RESERVE = 2_000_000_000_000_000n; // 0.002 HYPE kept for Elysium gas by "Max"

  let amount = $state('');
  let editingRecipient = $state(false);
  let recipientInput = $state('');
  let preview = $state<Preview | null>(null);
  let previewing = $state(false);
  let submitting = $state(false);

  let active = $state<ActiveExit | null>(null);
  let exit = $state<Exit | null>(null);

  let exits = $state<Exit[] | null>(null);
  let historyError = $state<string | null>(null);

  const amountWei = $derived(parseAmount(amount));
  const balance = $derived(app.balances[ELYSIUM_CHAIN_ID]);
  const claimTip = $derived(BigInt(app.config?.defaultClaimTip ?? '500000000000000'));

  const recipientValid = $derived(!editingRecipient || isAddress(recipientInput.trim()));
  const recipient: Address | null = $derived(
    editingRecipient ? (recipientValid ? getAddress(recipientInput.trim()) : null) : wallet.account,
  );
  const isSelf = $derived(!!recipient && recipient === wallet.account);
  const payout = $derived(amountWei && amountWei > claimTip ? amountWei - claimTip : 0n);

  const previewPlaceholder = $derived(
    wallet.status !== 'connected'
      ? 'Connect a wallet to compute the receiver address for your exit.'
      : !recipient
        ? 'Enter a valid recipient to compute the receiver.'
        : !amountWei || amountWei <= claimTip
          ? 'Enter an amount to compute the receiver for this exit.'
          : null,
  );

  // Recompute the receiver whenever the exit parameters change.
  $effect(() => {
    const r = recipient;
    const wei = amountWei;
    const tip = claimTip;
    preview = null;
    previewing = false;
    if (wallet.status !== 'connected' || !r || !wei || wei <= tip) return;
    previewing = true;
    let cancelled = false;
    const timer = setTimeout(async () => {
      try {
        const p = await wallet.previewReceiver({ recipient: r, amountWei: wei, claimTip: tip, toCore: false });
        if (!cancelled) preview = p;
      } catch (err) {
        if (!cancelled) toasts.push({ tone: 'error', title: "Couldn't compute the receiver", body: errorMessage(err) });
      } finally {
        if (!cancelled) previewing = false;
      }
    }, 300);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  });

  async function loadHistory(address: Address) {
    historyError = null;
    try {
      exits = await ferry.getExits(address);
    } catch (err) {
      historyError = errorMessage(err);
    }
  }

  $effect(() => {
    const account = wallet.account;
    exits = null;
    if (!account) return;
    loadHistory(account);
    const id = setInterval(() => loadHistory(account), 15_000);
    return () => clearInterval(id);
  });

  // Follow the active exit until delivered.
  $effect(() => {
    if (!active) return;
    const receiver = active.receiver;
    let stopped = false;
    let timer: ReturnType<typeof setTimeout>;
    const tick = async () => {
      try {
        const next = await ferry.getExit(receiver);
        if (stopped) return;
        const deliveredNow = exit?.stage !== 'delivered' && next.stage === 'delivered';
        const changed = exit?.stage !== next.stage;
        exit = next;
        if (changed && wallet.account) loadHistory(wallet.account);
        if (deliveredNow) {
          toasts.push({ tone: 'success', title: 'HYPE delivered on HyperEVM', body: `${formatHype(BigInt(next.params.amount) - BigInt(next.params.claimTip))} HYPE paid to ${shortAddress(next.params.recipient)}.` });
          app.refreshBalances();
          return;
        }
      } catch {
        /* not indexed yet */
      }
      if (!stopped) timer = setTimeout(tick, 2000);
    };
    tick();
    return () => {
      stopped = true;
      clearTimeout(timer);
    };
  });

  type Cta = { label: string; action?: () => void; disabled?: boolean; loading?: boolean };
  const cta: Cta = $derived.by(() => {
    if (wallet.status === 'connecting') return { label: 'Connecting', loading: true };
    if (wallet.status !== 'connected') return { label: 'Connect wallet', action: () => (app.walletDialogOpen = true) };
    if (wallet.chainId !== ELYSIUM_CHAIN_ID) return { label: 'Switch to Elysium', action: switchChain };
    if (!amountWei || amountWei === 0n) return { label: 'Enter an amount', disabled: true };
    if (!recipient) return { label: 'Enter a valid recipient', disabled: true };
    if (amountWei <= claimTip) return { label: 'Amount must exceed the keeper tip', disabled: true };
    if (balance !== null && amountWei > balance) return { label: 'Not enough HYPE on Elysium', disabled: true };
    if (submitting) return { label: 'Confirm in your wallet', loading: true };
    if (!preview || previewing) return { label: 'Computing receiver', loading: true };
    return { label: 'Ferry out', action: submit };
  });

  async function switchChain() {
    try {
      await wallet.switchChain(ELYSIUM_CHAIN_ID);
    } catch (err) {
      toasts.push({ tone: 'error', title: "Couldn't switch network", body: errorMessage(err) });
    }
  }

  async function submit() {
    if (!recipient || !amountWei || !preview) return;
    submitting = true;
    const r = recipient;
    try {
      const { txHash, receiver } = await wallet.ferryOut({ recipient: r, amountWei, claimTip, toCore: false });
      exit = null;
      active = { receiver, txHash, amount: amountWei, claimTip, recipient: r, startedAt: Date.now() / 1000 };
      toasts.push({ tone: 'info', title: 'Exit started', body: 'The keeper takes it from here. You can close this tab.' });
      app.refreshBalances();
      if (wallet.account) loadHistory(wallet.account);
    } catch (err) {
      toasts.push({ tone: 'error', title: 'Ferry Out not sent', body: errorMessage(err) });
    } finally {
      submitting = false;
    }
  }

  function openExit(e: Exit) {
    exit = e;
    active = {
      receiver: e.receiver,
      txHash: e.elysiumTx,
      amount: BigInt(e.params.amount),
      claimTip: BigInt(e.params.claimTip),
      recipient: e.params.recipient,
      startedAt: e.startedAt,
    };
    document.getElementById('out-main')?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }

  function reset() {
    active = null;
    exit = null;
    amount = '';
  }

  function max() {
    if (balance === null) return;
    amount = toInputValue(balance > GAS_RESERVE ? balance - GAS_RESERVE : 0n, 18, 4);
  }

  function editRecipient() {
    recipientInput = wallet.account ?? '';
    editingRecipient = true;
  }

  const crossing = $derived.by(() => {
    if (!active) return { progress: 0, status: 'idle' as const, label: 'The ferry is docked on Elysium, ready to carry HYPE back to HyperEVM.' };
    const stage = exit?.stage ?? 'intent';
    return {
      progress: outProgress(exit, clock.now, active.startedAt),
      status: stage === 'delivered' ? ('arrived' as const) : ('moving' as const),
      label: `Ferry Out crossing: ${stage}.`,
    };
  });

  const notes = [
    { term: 'One signature', text: 'You sign once on Elysium. You never need HyperEVM gas and never come back to claim.' },
    { term: '~25 minutes', text: 'Withdrawals mature on testnet in about 23-25 minutes. The keeper claims the moment it can.' },
    { term: 'Can’t be redirected', text: 'The receiver is fixed by your exit’s parameters before you sign, and it only ever pays your recipient.' },
    { term: 'Small tip', text: 'The keeper keeps the claim tip for running the HyperEVM transaction. The rest is yours.' },
  ];
</script>

<FlowLayout
  title="Leave Elysium. Nothing to claim later."
  lede="Sign once on Elysium. When the withdrawal matures, Ferryman's keeper claims it on HyperEVM and pays your wallet. No second visit, no HyperEVM gas."
>
  {#snippet eyebrow()}Ferry Out · Elysium <Icon name="arrow-right" size={12} /> HyperEVM{/snippet}

  {#snippet river()}
    <Crossing direction="out" progress={crossing.progress} status={crossing.status} stops={OUT_STOPS} label={crossing.label} />
  {/snippet}

  {#snippet main()}
    <div id="out-main">
      {#if active}
        <OutTracker {active} {exit} onreset={reset} />
      {:else}
        <form class="card form" onsubmit={(e) => (e.preventDefault(), cta.action?.())}>
          <AmountField id="ferry-out-amount" label="You send" bind:value={amount} {balance} onmax={max}>
            {#snippet side()}<AssetPill symbol="HYPE" chain="Elysium" />{/snippet}
          </AmountField>

          <div class="recipient" class:invalid={editingRecipient && recipientInput && !recipientValid}>
            <div class="r-top">
              {#if editingRecipient}
                <label for="recipient">Recipient on HyperEVM</label>
              {:else}
                <span class="label" id="recipient-label">Recipient on HyperEVM</span>
              {/if}
              {#if wallet.account}
                {#if editingRecipient}
                  <button type="button" class="link" onclick={() => (editingRecipient = false)}>Use my address</button>
                {:else}
                  <button type="button" class="link" onclick={editRecipient}>Change</button>
                {/if}
              {/if}
            </div>
            {#if editingRecipient}
              <input
                id="recipient"
                class="mono"
                bind:value={recipientInput}
                placeholder="0x…"
                autocomplete="off"
                spellcheck="false"
                aria-invalid={!recipientValid || undefined}
                aria-describedby="recipient-help"
              />
              <p id="recipient-help" class="help" class:error={recipientInput && !recipientValid}>
                {recipientInput && !recipientValid ? 'That isn’t a valid address. Check for a missing character.' : 'Paste any HyperEVM address. It cannot be changed after you sign.'}
              </p>
            {:else}
              <p class="r-value" aria-labelledby="recipient-label">
                {#if wallet.account}
                  <span class="mono">{shortAddress(wallet.account, 6, 6)}</span><span class="you">You</span>
                {:else}
                  <span class="faint">Your connected address</span>
                {/if}
              </p>
            {/if}
          </div>

          <ReceiverPreview {preview} loading={previewing} placeholder={previewPlaceholder} {recipient} {isSelf} />

          <dl class="summary">
            <div>
              <dt>You receive on HyperEVM</dt>
              <dd class="num strong">{formatHype(payout)} HYPE</dd>
            </div>
            <div>
              <dt>
                Keeper tip
                <Tooltip text="Paid to whoever runs the claim on HyperEVM, covering its gas. It comes out of the amount you send." />
              </dt>
              <dd class="num">{formatHype(claimTip)} HYPE</dd>
            </div>
            <div>
              <dt>Arrives</dt>
              <dd>{formatDuration(1500)} · no second signature</dd>
            </div>
          </dl>

          <Button type="submit" variant="primary" size="lg" full disabled={cta.disabled} loading={cta.loading}>
            {cta.label}
          </Button>
        </form>
      {/if}
    </div>
  {/snippet}

  {#snippet aside()}
    <Notes title="The crossing" items={notes} />
  {/snippet}

  {#snippet after()}
    <ExitHistory {exits} error={historyError} account={wallet.account} current={active?.receiver ?? null} onopen={openExit} onretry={() => wallet.account && loadHistory(wallet.account)} />
  {/snippet}
</FlowLayout>

<style>
  #out-main {
    scroll-margin-top: 96px;
  }

  .form {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
    padding: var(--s-3);
  }

  .recipient {
    padding: var(--s-4) var(--s-5);
    border-radius: var(--r-lg);
    border: 1px solid var(--line);
    transition: border-color var(--dur) var(--ease-out);
  }

  .recipient:focus-within {
    border-color: var(--line-strong);
  }

  .recipient.invalid {
    border-color: color-mix(in srgb, var(--danger) 55%, transparent);
  }

  .r-top {
    display: flex;
    justify-content: space-between;
    align-items: center;
    font-size: var(--fs-sm);
  }

  label,
  .label {
    color: var(--ink-2);
  }

  .link {
    color: var(--lantern-ink);
    font-size: var(--fs-sm);
    font-weight: 500;
  }

  .link:hover {
    text-decoration: underline;
    text-underline-offset: 0.2em;
  }

  .r-value {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    margin-top: var(--s-2);
    font-size: var(--fs-md);
  }

  .you {
    padding: 2px 8px;
    border-radius: var(--r-full);
    background: var(--bg-sunken);
    color: var(--ink-2);
    font-size: var(--fs-xs);
    font-weight: 500;
  }

  input {
    width: 100%;
    margin-top: var(--s-2);
    padding: 0;
    border: 0;
    background: none;
    font-size: var(--fs-base);
  }

  input:focus-visible {
    outline: none;
  }

  .help {
    margin-top: var(--s-2);
    font-size: var(--fs-xs);
    color: var(--ink-3);
  }

  .help.error {
    color: var(--danger);
  }

  .summary {
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: var(--s-4) var(--s-2) var(--s-2);
  }

  .summary div {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: var(--s-4);
    font-size: var(--fs-sm);
  }

  dt {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    color: var(--ink-2);
  }

  dd {
    margin: 0;
    text-align: right;
  }

  .strong {
    font-weight: 600;
  }

  .form :global(.btn.lg) {
    margin-top: var(--s-2);
  }

  @media (max-width: 560px) {
    .form {
      padding: var(--s-2);
    }

    .recipient {
      padding: var(--s-3) var(--s-4);
    }
  }
</style>
