<script lang="ts">
  import { app } from '../../app.svelte';
  import { explorerAddress } from '../../chains';
  import { formatHype, shortAddress } from '../../format';
  import { wallet } from '../../services';
  import { ELYSIUM_CHAIN_ID, HYPEREVM_CHAIN_ID } from '../../services/types';
  import Button from '../ui/Button.svelte';
  import CopyButton from '../ui/CopyButton.svelte';
  import ExternalLink from '../ui/ExternalLink.svelte';
  import Icon from '../ui/Icon.svelte';
  import Popover from '../ui/Popover.svelte';
  import TokenAvatar from '../ui/TokenAvatar.svelte';

  let open = $state(false);

  const rows = [
    { id: HYPEREVM_CHAIN_ID, label: 'HyperEVM' },
    { id: ELYSIUM_CHAIN_ID, label: 'Elysium' },
  ] as const;

  async function disconnect() {
    open = false;
    await wallet.disconnect();
  }
</script>

{#if wallet.status === 'connected' && wallet.account}
  {@const account = wallet.account}
  <Popover bind:open>
    {#snippet trigger({ toggle, expanded })}
      <button class="account" onclick={toggle} aria-expanded={expanded} aria-haspopup="true" aria-label="Wallet {account}">
        <TokenAvatar symbol="" address={account} size={22} />
        <span class="mono">{shortAddress(account)}</span>
      </button>
    {/snippet}
    <div class="sheet">
      <div class="who">
        <TokenAvatar symbol="" address={account} size={36} />
        <div class="addr">
          <span class="mono">{shortAddress(account, 6, 6)}</span>
          <CopyButton value={account} label="Copy address" />
        </div>
      </div>
      <dl>
        {#each rows as r (r.id)}
          <div>
            <dt>{r.label}</dt>
            <dd class="num">
              {app.balances[r.id] === null ? '—' : formatHype(app.balances[r.id] as bigint)} <span class="faint">HYPE</span>
            </dd>
          </div>
        {/each}
      </dl>
      <div class="links">
        <ExternalLink href={explorerAddress(HYPEREVM_CHAIN_ID, account)}>HyperEVM explorer</ExternalLink>
        <ExternalLink href={explorerAddress(ELYSIUM_CHAIN_ID, account)}>Elysium explorer</ExternalLink>
      </div>
      <button class="disconnect" onclick={disconnect}><Icon name="logout" size={16} />Disconnect</button>
    </div>
  </Popover>
{:else}
  <Button variant="primary" loading={wallet.status === 'connecting'} onclick={() => (app.walletDialogOpen = true)}>
    {#if wallet.status !== 'connecting'}<Icon name="wallet" size={16} />{/if}
    {wallet.status === 'connecting' ? 'Connecting' : 'Connect'}
  </Button>
{/if}

<style>
  .account {
    display: inline-flex;
    align-items: center;
    gap: var(--s-2);
    height: 40px;
    padding: 0 var(--s-3) 0 9px;
    border-radius: var(--r-full);
    border: 1px solid var(--line-2);
    background: var(--bg-raised);
    font-size: var(--fs-sm);
    transition: border-color var(--dur) var(--ease-out);
  }

  .account:hover {
    border-color: var(--line-strong);
  }

  .account:focus-visible {
    border-radius: var(--r-full);
  }

  .sheet {
    width: 272px;
    padding: var(--s-2);
  }

  .who {
    display: flex;
    align-items: center;
    gap: var(--s-3);
    padding-bottom: var(--s-3);
  }

  .addr {
    display: flex;
    align-items: center;
    gap: var(--s-1);
    font-size: var(--fs-sm);
  }

  dl {
    display: grid;
    gap: var(--s-2);
    padding: var(--s-3) 0;
    border-top: 1px solid var(--line);
  }

  dl div {
    display: flex;
    justify-content: space-between;
    font-size: var(--fs-sm);
  }

  dt {
    color: var(--ink-2);
  }

  .links {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s-1) var(--s-4);
    padding: var(--s-3) 0;
    border-top: 1px solid var(--line);
  }

  .disconnect {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    width: 100%;
    padding: var(--s-3) 0 var(--s-1);
    border-top: 1px solid var(--line);
    color: var(--ink-2);
    font-size: var(--fs-sm);
  }

  .disconnect:hover {
    color: var(--danger);
  }

  @media (max-width: 560px) {
    .account .mono {
      display: none;
    }

    .account {
      padding: 0 9px;
    }
  }
</style>
