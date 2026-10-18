<script lang="ts">
  import { app } from '../../app.svelte';
  import { isDemo, wallet } from '../../services';
  import { errorMessage, toasts } from '../../toasts.svelte';
  import Dialog from '../ui/Dialog.svelte';
  import Icon from '../ui/Icon.svelte';
  import Notice from '../ui/Notice.svelte';
  import Spinner from '../ui/Spinner.svelte';

  let pending = $state<string | null>(null);

  $effect(() => {
    if (app.walletDialogOpen) wallet.discover();
  });

  async function connect(id: string) {
    pending = id;
    try {
      await wallet.connect(id);
      app.walletDialogOpen = false;
    } catch (err) {
      toasts.push({ tone: 'error', title: "Couldn't connect", body: errorMessage(err) });
    } finally {
      pending = null;
    }
  }
</script>

<Dialog bind:open={app.walletDialogOpen} title="Connect a wallet" description="Ferryman never holds your funds. Every crossing is signed by you.">
  {#if wallet.providers.length}
    <ul class="list">
      {#each wallet.providers as p (p.id)}
        <li>
          <button class="provider" onclick={() => connect(p.id)} disabled={pending !== null}>
            {#if p.icon}
              <img src={p.icon} alt="" width="36" height="36" />
            {:else}
              <span class="fallback"><Icon name="wallet" size={18} /></span>
            {/if}
            <span class="text">
              <span class="name">{p.name}</span>
              <span class="rdns mono">{p.rdns}</span>
            </span>
            {#if pending === p.id}<Spinner size={16} />{:else}<Icon name="arrow-right" size={16} />{/if}
          </button>
        </li>
      {/each}
    </ul>
  {:else}
    <div class="empty">
      <p>No browser wallet found.</p>
      <p class="faint">Install a wallet that supports EIP-6963 (Rabby, MetaMask and most others do), then reload.</p>
    </div>
  {/if}
  {#if isDemo}
    <div class="demo-note">
      <Notice>These are simulated wallets for the demo. Nothing is signed and no funds move.</Notice>
    </div>
  {/if}
</Dialog>

<style>
  .list {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }

  .provider {
    display: flex;
    align-items: center;
    gap: var(--s-3);
    width: 100%;
    padding: var(--s-3);
    border: 1px solid var(--line);
    border-radius: var(--r-lg);
    text-align: left;
    transition: border-color var(--dur) var(--ease-out), background-color var(--dur) var(--ease-out);
  }

  .provider:hover:not(:disabled) {
    border-color: var(--line-strong);
    background: var(--bg-hover);
  }

  .provider:disabled {
    opacity: 0.7;
    cursor: progress;
  }

  .provider :global(.icon) {
    color: var(--ink-3);
  }

  img,
  .fallback {
    width: 36px;
    height: 36px;
    border-radius: var(--r-md);
  }

  .fallback {
    display: grid;
    place-items: center;
    background: var(--bg-sunken);
  }

  .text {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-width: 0;
  }

  .name {
    font-weight: 500;
  }

  .rdns {
    color: var(--ink-3);
    font-size: var(--fs-xs);
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .empty {
    padding: var(--s-6) 0;
    text-align: center;
    font-size: var(--fs-sm);
  }

  .demo-note {
    margin-top: var(--s-4);
  }
</style>
