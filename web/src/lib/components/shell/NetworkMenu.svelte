<script lang="ts">
  import { CHAIN_LABEL, isSupportedChain } from '../../chains';
  import { wallet } from '../../services';
  import { ELYSIUM_CHAIN_ID, HYPEREVM_CHAIN_ID, type ChainId } from '../../services/types';
  import { errorMessage, toasts } from '../../toasts.svelte';
  import Icon from '../ui/Icon.svelte';
  import Popover from '../ui/Popover.svelte';
  import Spinner from '../ui/Spinner.svelte';

  let open = $state(false);
  let switching = $state<ChainId | null>(null);

  const chains: { id: ChainId; note: string }[] = [
    { id: HYPEREVM_CHAIN_ID, note: 'Ferry In starts here' },
    { id: ELYSIUM_CHAIN_ID, note: 'Ferry Out starts here' },
  ];

  const current = $derived(wallet.chainId);
  const supported = $derived(isSupportedChain(current));

  async function select(id: ChainId) {
    if (id === current) return (open = false);
    switching = id;
    try {
      await wallet.switchChain(id);
      open = false;
    } catch (err) {
      toasts.push({ tone: 'error', title: `Couldn't switch to ${CHAIN_LABEL[id]}`, body: errorMessage(err) });
    } finally {
      switching = null;
    }
  }

  async function addElysium() {
    try {
      await wallet.addElysiumNetwork();
      toasts.push({ tone: 'success', title: 'Elysium added to your wallet' });
    } catch (err) {
      toasts.push({ tone: 'error', title: "Couldn't add Elysium", body: errorMessage(err) });
    }
  }
</script>

{#if wallet.status !== 'connected'}
  <span class="pill idle"><span class="dot" aria-hidden="true"></span>Testnet</span>
{:else}
  <Popover bind:open>
    {#snippet trigger({ toggle, expanded })}
      <button
        class="pill"
        class:wrong={!supported}
        onclick={toggle}
        aria-expanded={expanded}
        aria-haspopup="true"
        aria-label="Network: {supported ? CHAIN_LABEL[current as ChainId] : 'unsupported'}. Change network"
      >
        <span class="dot" data-chain={current} aria-hidden="true"></span>
        <span class="label">{supported ? CHAIN_LABEL[current as ChainId] : 'Wrong network'}</span>
        <Icon name="chevron-down" size={14} />
      </button>
    {/snippet}
    <p class="eyebrow head">Network</p>
    <ul>
      {#each chains as c (c.id)}
        <li>
          <button class="option" class:selected={c.id === current} onclick={() => select(c.id)} disabled={switching !== null}>
            <span class="dot" data-chain={c.id} aria-hidden="true"></span>
            <span class="text">
              <span>{CHAIN_LABEL[c.id]} <span class="faint mono">{c.id}</span></span>
              <span class="note">{c.note}</span>
            </span>
            {#if switching === c.id}<Spinner size={14} />{:else if c.id === current}<Icon name="check" size={16} />{/if}
          </button>
        </li>
      {/each}
    </ul>
    <button class="add" onclick={addElysium}><Icon name="plus" size={15} />Add Elysium to wallet</button>
  </Popover>
{/if}

<style>
  .pill {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    height: 36px;
    padding: 0 10px 0 12px;
    border-radius: var(--r-full);
    border: 1px solid var(--line-2);
    color: var(--ink);
    font-size: var(--fs-sm);
    font-weight: 500;
    white-space: nowrap;
    transition: border-color var(--dur) var(--ease-out), background-color var(--dur) var(--ease-out);
  }

  button.pill:hover {
    border-color: var(--line-strong);
    background: var(--bg-hover);
  }

  .pill.idle {
    padding-right: 12px;
    color: var(--ink-2);
    border-style: dashed;
  }

  .pill.wrong {
    color: var(--danger);
    border-color: color-mix(in srgb, var(--danger) 40%, transparent);
    background: var(--danger-soft);
  }

  .pill :global(.icon) {
    color: var(--ink-3);
  }

  .dot {
    flex: none;
    width: 8px;
    height: 8px;
    border-radius: var(--r-full);
    background: var(--ink-3);
  }

  .dot[data-chain='998'] {
    background: var(--river);
  }

  .dot[data-chain='99801'] {
    background: var(--lantern);
  }

  .wrong .dot {
    background: var(--danger);
  }

  .head {
    padding: var(--s-2) var(--s-3) var(--s-1);
  }

  .option {
    display: flex;
    align-items: center;
    gap: var(--s-3);
    width: 100%;
    padding: var(--s-2) var(--s-3);
    border-radius: var(--r-md);
    text-align: left;
    font-size: var(--fs-sm);
  }

  .option:hover:not(:disabled) {
    background: var(--bg-hover);
  }

  .option.selected {
    background: var(--bg-sunken);
  }

  .text {
    display: flex;
    flex-direction: column;
    flex: 1;
    font-weight: 500;
  }

  .note {
    color: var(--ink-3);
    font-size: var(--fs-xs);
    font-weight: 400;
  }

  .add {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    width: 100%;
    margin-top: var(--s-1);
    padding: var(--s-3);
    border-top: 1px solid var(--line);
    color: var(--ink-2);
    font-size: var(--fs-sm);
  }

  .add:hover {
    color: var(--ink);
  }

  @media (max-width: 560px) {
    .pill {
      height: 38px;
    }
  }
</style>
