<script lang="ts">
  import { getAddress, isAddress } from 'viem';
  import { explorerTx } from '../../chains';
  import { formatAge, formatHype, formatInt, shortAddress } from '../../format';
  import { ferry, wallet } from '../../services';
  import { ELYSIUM_CHAIN_ID, HYPEREVM_CHAIN_ID, type Address, type Hash, type StuckWithdrawal } from '../../services/types';
  import { errorMessage, toasts } from '../../toasts.svelte';
  import Badge from '../ui/Badge.svelte';
  import Button from '../ui/Button.svelte';
  import ExternalLink from '../ui/ExternalLink.svelte';
  import Icon from '../ui/Icon.svelte';
  import Notice from '../ui/Notice.svelte';
  import Skeleton from '../ui/Skeleton.svelte';

  type ClaimState = { phase: 'sending' | 'queued' | 'claimed' | 'failed'; txHash?: Hash | null };

  let input = $state('');
  let touched = $state(false);
  let searched = $state<Address | null>(null);
  let results = $state<StuckWithdrawal[] | null>(null);
  let loading = $state(false);
  let error = $state<string | null>(null);
  let claims = $state<Record<number, ClaimState>>({});

  const valid = $derived(isAddress(input.trim()));
  const showInvalid = $derived(touched && input.trim().length > 0 && !valid);
  const claimableTotal = $derived(
    (results ?? []).filter((w) => w.claimable && !claims[w.messageId]).reduce((sum, w) => sum + BigInt(w.amount), 0n),
  );

  async function lookup(address?: string) {
    if (address) input = address;
    touched = true;
    if (!isAddress(input.trim())) return;
    const target = getAddress(input.trim());
    searched = target;
    loading = true;
    error = null;
    results = null;
    claims = {};
    try {
      results = await ferry.lookupStuck(target);
    } catch (err) {
      error = errorMessage(err);
    } finally {
      loading = false;
    }
  }

  async function claim(w: StuckWithdrawal) {
    claims[w.messageId] = { phase: 'sending' };
    try {
      let res = await ferry.claimForMe(w.messageId);
      if (res.status === 'not_claimable') {
        claims[w.messageId] = { phase: 'failed' };
        toasts.push({ tone: 'error', title: 'Not claimable yet', body: `Message #${formatInt(w.messageId)} hasn't matured, or was already claimed.` });
        return;
      }
      claims[w.messageId] = { phase: 'queued' };
      // The claim endpoint is idempotent: ask again until the keeper's transaction lands.
      while (res.status === 'queued') {
        await new Promise((r) => setTimeout(r, 2500));
        res = await ferry.claimForMe(w.messageId);
      }
      claims[w.messageId] = { phase: 'claimed', txHash: res.txHash };
      toasts.push({
        tone: 'success',
        title: `${formatHype(w.amount)} HYPE returned`,
        body: `Delivered to ${shortAddress(w.owner)} on HyperEVM.`,
        link: res.txHash ? { href: explorerTx(HYPEREVM_CHAIN_ID, res.txHash), label: 'View claim transaction' } : undefined,
      });
    } catch (err) {
      claims[w.messageId] = { phase: 'failed' };
      toasts.push({ tone: 'error', title: 'Claim failed', body: errorMessage(err) });
    }
  }

  function statusOf(w: StuckWithdrawal): { label: string; tone: 'success' | 'lantern' | 'river' | 'neutral' } {
    const c = claims[w.messageId];
    if (c?.phase === 'claimed' || w.status === 'relayed') return { label: 'Returned', tone: 'success' };
    if (c?.phase === 'queued' || c?.phase === 'sending') return { label: 'Claiming', tone: 'river' };
    if (w.claimable) return { label: 'Claimable', tone: 'lantern' };
    return { label: 'Maturing', tone: 'neutral' };
  }
</script>

<section class="card lookup" aria-labelledby="lookup-title">
  <div class="intro">
    <h2 id="lookup-title">Find stuck withdrawals</h2>
    <p class="muted">Paste any address. If an Elysium withdrawal to it matured and nobody claimed it, the keeper will claim it to that same address, free.</p>
  </div>

  <form class="search" onsubmit={(e) => (e.preventDefault(), lookup())} novalidate>
    <label for="lookup-address" class="visually-hidden">Address to check</label>
    <div class="input" class:invalid={showInvalid}>
      <Icon name="search" size={17} />
      <input
        id="lookup-address"
        class="mono"
        bind:value={input}
        onblur={() => (touched = true)}
        placeholder="0x… owner address"
        autocomplete="off"
        spellcheck="false"
        aria-invalid={showInvalid || undefined}
        aria-describedby="lookup-help"
      />
    </div>
    <Button type="submit" variant="primary" {loading}>Look up</Button>
  </form>
  <p id="lookup-help" class="help" class:error={showInvalid}>
    {#if showInvalid}
      That isn't a valid address.
    {:else if wallet.account}
      <button type="button" class="link" onclick={() => wallet.account && lookup(wallet.account)}>Check my address</button>
      <span class="faint">· {shortAddress(wallet.account)}</span>
    {:else}
      Funds always go to the original owner. Ferryman can't redirect them.
    {/if}
  </p>

  <div class="results" aria-live="polite" aria-busy={loading}>
    {#if loading}
      <ul>
        {#each Array(3) as _, i (i)}
          <li class="item sk"><Skeleton width="28%" height="22px" /><Skeleton width="96px" height="32px" radius="var(--r-sm)" /></li>
        {/each}
      </ul>
    {:else if error}
      <Notice tone="danger">Lookup failed: {error}. <button type="button" class="link" onclick={() => lookup()}>Try again</button></Notice>
    {:else if results && searched}
      <div class="summary">
        <span>
          {#if results.length === 0}
            Nothing stuck for <span class="mono">{shortAddress(searched)}</span>.
          {:else}
            {results.length} {results.length === 1 ? 'withdrawal' : 'withdrawals'} for <span class="mono">{shortAddress(searched)}</span>
          {/if}
        </span>
        {#if claimableTotal > 0n}<span class="faint num">{formatHype(claimableTotal)} HYPE claimable</span>{/if}
      </div>
      {#if results.length === 0}
        <p class="empty">Every withdrawal to this address has been claimed, or there never were any. Nothing to rescue.</p>
      {:else}
        <ul>
          {#each results as w (w.messageId)}
            {@const s = statusOf(w)}
            {@const c = claims[w.messageId]}
            <li class="item">
              <div class="what">
                <span class="amount num">{formatHype(w.amount)} <span class="unit">HYPE</span></span>
                <span class="meta">
                  <a class="mono" href={explorerTx(ELYSIUM_CHAIN_ID, w.originTx)} target="_blank" rel="noopener noreferrer">#{formatInt(w.messageId)}<span class="visually-hidden"> origin transaction (opens explorer)</span></a>
                  · {formatAge(w.ageSeconds)} old
                </span>
              </div>
              <Badge tone={s.tone}>{s.label}</Badge>
              <div class="action">
                {#if c?.phase === 'claimed' || w.status === 'relayed'}
                  {#if c?.txHash}
                    <ExternalLink href={explorerTx(HYPEREVM_CHAIN_ID, c.txHash)}>Claim tx</ExternalLink>
                  {:else}
                    <span class="done"><Icon name="check" size={15} />Delivered</span>
                  {/if}
                {:else if c?.phase === 'sending' || c?.phase === 'queued'}
                  <Button size="sm" loading disabled>{c.phase === 'sending' ? 'Requesting' : 'Keeper claiming'}</Button>
                {:else if w.claimable}
                  <Button size="sm" variant="secondary" onclick={() => claim(w)}>{c?.phase === 'failed' ? 'Retry' : 'Claim for me'}</Button>
                {:else}
                  <span class="faint wait">Not matured yet</span>
                {/if}
              </div>
            </li>
          {/each}
        </ul>
      {/if}
    {:else}
      <p class="empty hint">
        Try any address that has used the Elysium faucet or quests. Most have a few thousandths of a HYPE waiting.
      </p>
    {/if}
  </div>
</section>

<style>
  .lookup {
    padding: var(--s-6);
  }

  .intro h2 {
    font-size: var(--fs-xl);
  }

  .intro p {
    max-width: 52ch;
    margin-top: var(--s-2);
    font-size: var(--fs-sm);
  }

  .search {
    display: flex;
    gap: var(--s-2);
    margin-top: var(--s-5);
  }

  .input {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    flex: 1;
    min-width: 0;
    height: 40px;
    padding: 0 var(--s-3);
    border-radius: var(--r-md);
    background: var(--bg-sunken);
    border: 1px solid transparent;
    color: var(--ink-3);
    transition: border-color var(--dur) var(--ease-out);
  }

  .input:focus-within {
    border-color: var(--line-strong);
  }

  .input.invalid {
    border-color: color-mix(in srgb, var(--danger) 55%, transparent);
  }

  .input input {
    flex: 1;
    min-width: 0;
    border: 0;
    background: none;
    color: var(--ink);
    font-size: var(--fs-sm);
  }

  .input input:focus-visible {
    outline: none;
  }

  .help {
    min-height: 20px;
    margin-top: var(--s-2);
    font-size: var(--fs-xs);
    color: var(--ink-3);
  }

  .help.error {
    color: var(--danger);
  }

  .link {
    color: var(--lantern-ink);
    font-weight: 500;
    font-size: inherit;
    text-decoration: underline;
    text-decoration-color: transparent;
    text-underline-offset: 0.2em;
  }

  .link:hover {
    text-decoration-color: currentColor;
  }

  .results {
    margin-top: var(--s-5);
    padding-top: var(--s-4);
    border-top: 1px solid var(--line);
  }

  .summary {
    display: flex;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: var(--s-2);
    padding-bottom: var(--s-2);
    font-size: var(--fs-sm);
  }

  .item {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto 140px;
    align-items: center;
    gap: var(--s-4);
    padding: var(--s-3) 0;
    border-bottom: 1px solid var(--line);
  }

  .item:last-child {
    border-bottom: 0;
  }

  .item.sk {
    grid-template-columns: 1fr auto;
  }

  .what {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .amount {
    font-family: var(--font-display);
    font-size: var(--fs-lg);
    font-variation-settings: 'opsz' 36;
    letter-spacing: -0.01em;
    line-height: 1.3;
  }

  .unit {
    font-family: var(--font-sans);
    font-size: var(--fs-xs);
    color: var(--ink-3);
    letter-spacing: 0;
  }

  .meta {
    font-size: var(--fs-xs);
    color: var(--ink-3);
  }

  .meta a {
    color: var(--ink-2);
    text-decoration: none;
  }

  .meta a:hover {
    text-decoration: underline;
  }

  .action {
    display: flex;
    justify-content: flex-end;
  }

  .done {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    color: var(--success);
    font-size: var(--fs-sm);
  }

  .wait {
    font-size: var(--fs-xs);
  }

  .empty {
    padding: var(--s-4) 0 var(--s-2);
    color: var(--ink-2);
    font-size: var(--fs-sm);
  }

  .hint {
    color: var(--ink-3);
  }

  @media (max-width: 560px) {
    .lookup {
      padding: var(--s-5) var(--s-4);
    }

    .item {
      grid-template-columns: minmax(0, 1fr) auto;
      grid-template-areas:
        'what badge'
        'action action';
      row-gap: var(--s-2);
    }

    .what {
      grid-area: what;
    }

    .item :global(.badge) {
      grid-area: badge;
    }

    .action {
      grid-area: action;
      justify-content: stretch;
    }

    .action :global(.btn) {
      width: 100%;
    }
  }
</style>
