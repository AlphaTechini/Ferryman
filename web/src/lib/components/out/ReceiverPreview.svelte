<script lang="ts">
  import { shortAddress } from '../../format';
  import type { Address, ReceiverPreview } from '../../services/types';
  import CopyButton from '../ui/CopyButton.svelte';
  import Icon from '../ui/Icon.svelte';
  import Skeleton from '../ui/Skeleton.svelte';
  import Tooltip from '../ui/Tooltip.svelte';

  let {
    preview,
    loading,
    placeholder,
    recipient,
    isSelf,
  }: {
    preview: ReceiverPreview | null;
    loading: boolean;
    placeholder: string | null;
    recipient: Address | null;
    isSelf: boolean;
  } = $props();

  /** 0x + groups of four: easier to compare against the wallet prompt. */
  const groups = (a: string) => a.slice(2).match(/.{1,4}/g) ?? [];
</script>

<section class="receiver" aria-labelledby="receiver-title" aria-live="polite">
  <div class="head">
    <h3 id="receiver-title" class="eyebrow">Receiver for this exit</h3>
    <Tooltip
      label="What is the receiver?"
      text="A one-time contract address on HyperEVM, derived from this exact exit: you, the recipient, amount, tip and nonce. Your withdrawal is sent there, and it can only ever pay the recipient. Check that your wallet shows this address."
    />
  </div>

  {#if placeholder}
    <p class="placeholder">{placeholder}</p>
  {:else if loading || !preview}
    <div class="sk" aria-busy="true">
      <Skeleton width="92%" height="18px" />
      <Skeleton width="56%" height="18px" />
    </div>
  {:else}
    <div class="addr">
      <p class="mono" title={preview.receiver}>
        <span class="prefix">0x</span>{#each groups(preview.receiver) as g, i (i)}<span class="g">{g}</span>{/each}
      </p>
      <CopyButton value={preview.receiver} label="Copy receiver address" />
    </div>
    {#if recipient}
      <p class="guard">
        <Icon name="shield" size={14} />
        <span>
          Funds can only go to the recipient, <span class="mono">{shortAddress(recipient)}</span>{isSelf ? ' (you)' : ''}. Nobody, including
          Ferryman, can redirect them.
        </span>
      </p>
    {/if}
  {/if}
</section>

<style>
  .receiver {
    position: relative;
    padding: var(--s-4) var(--s-5);
    border-radius: var(--r-lg);
    background:
      linear-gradient(90deg, var(--lantern) 0 2px, transparent 2px) no-repeat,
      var(--lantern-soft);
  }

  .head {
    display: flex;
    align-items: center;
    gap: 4px;
  }

  h3 {
    color: var(--lantern-ink);
  }

  .addr {
    display: flex;
    align-items: flex-start;
    gap: var(--s-2);
    margin-top: var(--s-2);
  }

  .addr p {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-wrap: wrap;
    column-gap: 0.42em;
    font-size: 1.0625rem;
    line-height: 1.5;
    color: var(--ink);
  }

  .prefix {
    color: var(--ink-3);
    margin-right: -0.3em;
  }

  .guard {
    display: flex;
    gap: 6px;
    margin-top: var(--s-2);
    font-size: var(--fs-xs);
    line-height: 1.5;
    color: var(--ink-2);
  }

  .guard :global(.icon) {
    margin-top: 2px;
    color: var(--success);
  }

  .placeholder {
    margin-top: var(--s-2);
    font-size: var(--fs-sm);
    color: var(--ink-2);
  }

  .sk {
    display: flex;
    flex-direction: column;
    gap: 8px;
    margin-top: var(--s-3);
  }

  @media (max-width: 560px) {
    .receiver {
      padding: var(--s-3) var(--s-4);
    }

    .addr p {
      font-size: var(--fs-base);
    }
  }
</style>
