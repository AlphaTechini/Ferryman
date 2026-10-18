<script lang="ts">
  import { formatAmount, relativeTime, shortAddress } from '../../format';
  import type { Token } from '../../services/types';
  import Badge from '../ui/Badge.svelte';
  import Button from '../ui/Button.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import Icon from '../ui/Icon.svelte';
  import Skeleton from '../ui/Skeleton.svelte';
  import TokenAvatar from '../ui/TokenAvatar.svelte';
  import { parseEther } from 'viem';

  let {
    open = $bindable(false),
    tokens,
    error,
    selected,
    onselect,
    onretry,
  }: {
    open: boolean;
    tokens: Token[] | null;
    error: string | null;
    selected: Token | null;
    onselect: (t: Token) => void;
    onretry: () => void;
  } = $props();

  let query = $state('');
  let search = $state<HTMLInputElement | undefined>();

  $effect(() => {
    if (open) {
      query = '';
      queueMicrotask(() => search?.focus());
    }
  });

  const filtered = $derived.by(() => {
    const list = [...(tokens ?? [])].sort((a, b) => b.launchedAt - a.launchedAt);
    const q = query.trim().toLowerCase();
    if (!q) return list;
    return list.filter(
      (t) => t.symbol.toLowerCase().includes(q) || t.name.toLowerCase().includes(q) || t.address.toLowerCase().startsWith(q),
    );
  });

  /** Symbols that more than one launch uses: the look-alike case. */
  const duplicated = $derived.by(() => {
    const seen = new Map<string, number>();
    for (const t of tokens ?? []) seen.set(t.symbol, (seen.get(t.symbol) ?? 0) + 1);
    return new Set([...seen].filter(([, n]) => n > 1).map(([s]) => s));
  });

  const price = (t: Token) => (t.priceHype ? formatAmount(parseEther(t.priceHype), 18, { max: 7 }) : '—');
</script>

<Dialog bind:open title="Choose a token" description="Every token here comes from an AscendLaunchpad launch event. Names can be copied; addresses can't.">
  <div class="search">
    <Icon name="search" size={17} />
    <input
      bind:this={search}
      bind:value={query}
      type="search"
      placeholder="Search name, symbol or 0x address"
      aria-label="Search tokens"
      autocomplete="off"
      spellcheck="false"
    />
  </div>

  {#if error}
    <div class="state">
      <p>Couldn't load the token list.</p>
      <p class="faint">{error}</p>
      <Button size="sm" onclick={onretry}><Icon name="refresh" size={15} />Try again</Button>
    </div>
  {:else if !tokens}
    <ul class="list" aria-busy="true" aria-label="Loading tokens">
      {#each Array(5) as _, i (i)}
        <li class="row skeleton">
          <Skeleton width="36px" height="36px" radius="999px" />
          <span class="sk-text"><Skeleton width="45%" height="14px" /><Skeleton width="70%" height="11px" /></span>
        </li>
      {/each}
    </ul>
  {:else if filtered.length === 0}
    <div class="state">
      <p>No verified token matches “{query}”.</p>
      <p class="faint">Only tokens launched through AscendLaunchpad are listed. If someone sent you a link, compare the contract address.</p>
    </div>
  {:else}
    <p class="count faint" aria-live="polite">{filtered.length} verified {filtered.length === 1 ? 'launch' : 'launches'}</p>
    <ul class="list">
      {#each filtered as t (t.address)}
        <li>
          <button
            class="row"
            class:selected={selected?.address === t.address}
            onclick={() => onselect(t)}
            aria-label="{t.name} ({t.symbol}), contract {t.address}"
          >
            <TokenAvatar symbol={t.symbol} address={t.address} />
            <span class="main">
              <span class="name">
                {t.name}
                {#if t.ourLaunch}<Badge tone="lantern">Ferryman</Badge>{/if}
                {#if duplicated.has(t.symbol)}<Badge tone="danger">Shared name · check address</Badge>{/if}
              </span>
              <span class="meta">
                <span class="symbol">{t.symbol}</span>
                <span class="mono">{shortAddress(t.address, 6, 4)}</span>
                <span class="faint">{t.quoteSymbol} pool</span>
              </span>
              <span class="proof">
                <Icon name="shield" size={13} />
                Launched by <span class="mono">{shortAddress(t.launcher)}</span> · {relativeTime(t.launchedAt)}
              </span>
            </span>
            <span class="price">
              <span class="num">{price(t)}</span>
              <span class="faint">HYPE</span>
            </span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</Dialog>

<style>
  .search {
    position: sticky;
    top: 0;
    z-index: 1;
    display: flex;
    align-items: center;
    gap: var(--s-2);
    height: 46px;
    padding: 0 var(--s-4);
    border-radius: var(--r-md);
    background: var(--bg-sunken);
    border: 1px solid transparent;
    color: var(--ink-3);
    box-shadow: 0 -12px 0 var(--bg-raised);
  }

  .search:focus-within {
    border-color: var(--line-strong);
  }

  .search input {
    flex: 1;
    min-width: 0;
    border: 0;
    background: none;
    color: var(--ink);
    font-size: var(--fs-base);
  }

  .search input:focus-visible {
    outline: none;
  }

  .search input::-webkit-search-cancel-button {
    display: none;
  }

  .count {
    margin: var(--s-4) 0 var(--s-2);
    font-size: var(--fs-xs);
  }

  .list {
    display: flex;
    flex-direction: column;
    gap: 2px;
    margin: 0 calc(var(--s-2) * -1);
  }

  .row {
    display: flex;
    align-items: flex-start;
    gap: var(--s-3);
    width: 100%;
    padding: var(--s-3) var(--s-2);
    border-radius: var(--r-md);
    text-align: left;
    transition: background-color var(--dur-fast) var(--ease-out);
  }

  button.row:hover {
    background: var(--bg-hover);
  }

  .row.selected {
    background: var(--lantern-soft);
  }

  .row.skeleton {
    align-items: center;
  }

  .sk-text {
    display: flex;
    flex-direction: column;
    gap: 6px;
    flex: 1;
  }

  .main {
    display: flex;
    flex-direction: column;
    gap: 2px;
    flex: 1;
    min-width: 0;
  }

  .name {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
    font-weight: 500;
  }

  .meta {
    display: flex;
    gap: var(--s-2);
    font-size: var(--fs-sm);
    color: var(--ink-2);
  }

  .symbol {
    font-weight: 500;
  }

  .proof {
    display: flex;
    align-items: center;
    gap: 5px;
    margin-top: 2px;
    color: var(--ink-3);
    font-size: var(--fs-xs);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .proof :global(.icon) {
    color: var(--success);
  }

  .price {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    flex: none;
    font-size: var(--fs-sm);
    line-height: 1.4;
  }

  .price .faint {
    font-size: var(--fs-xs);
  }

  .state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--s-2);
    padding: var(--s-10) var(--s-4);
    text-align: center;
    font-size: var(--fs-sm);
  }

  @media (max-width: 560px) {
    .price {
      display: none;
    }

    .proof {
      white-space: normal;
      align-items: flex-start;
    }

    .proof :global(.icon) {
      margin-top: 2px;
    }
  }
</style>
