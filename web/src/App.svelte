<script lang="ts">
  import { onMount } from 'svelte';
  import { fade } from 'svelte/transition';
  import { prefersReducedMotion } from 'svelte/motion';
  import { app } from './lib/app.svelte';
  import FerryIn from './lib/components/in/FerryIn.svelte';
  import FerryOut from './lib/components/out/FerryOut.svelte';
  import Rescue from './lib/components/rescue/Rescue.svelte';
  import Footer from './lib/components/shell/Footer.svelte';
  import Header from './lib/components/shell/Header.svelte';
  import HowItWorks from './lib/components/shell/HowItWorks.svelte';
  import WalletDialog from './lib/components/shell/WalletDialog.svelte';
  import Toaster from './lib/components/ui/Toaster.svelte';
  import { ROUTES, router } from './lib/router.svelte';
  import { wallet } from './lib/services';

  onMount(() => {
    app.loadConfig();
  });

  // Balances follow the connected account and network.
  $effect(() => {
    void wallet.account;
    void wallet.chainId;
    app.refreshBalances();
  });

  const title = $derived(ROUTES.find((r) => r.id === router.route));
</script>

<svelte:head>
  <title>{title ? `${title.label === 'Rescue' ? 'Rescue' : `Ferry ${title.label}`} · Ferryman` : 'Ferryman'}</title>
</svelte:head>

<a class="skip" href="#main">Skip to content</a>
<Header />

<main id="main" tabindex="-1">
  {#key router.route}
    <div in:fade={{ duration: prefersReducedMotion.current ? 0 : 220 }}>
      {#if router.route === 'in'}
        <FerryIn />
      {:else if router.route === 'out'}
        <FerryOut />
      {:else}
        <Rescue />
      {/if}
    </div>
  {/key}
</main>

<Footer />
<WalletDialog />
<HowItWorks />
<Toaster />

<style>
  main {
    position: relative;
    z-index: 1;
    min-height: calc(100dvh - 68px - 220px);
  }

  main:focus {
    outline: none;
  }

  .skip {
    position: absolute;
    z-index: 100;
    top: 8px;
    left: 8px;
    padding: var(--s-2) var(--s-4);
    border-radius: var(--r-md);
    background: var(--bg-inverse);
    color: var(--ink-inverse);
    transform: translateY(-200%);
    transition: transform var(--dur) var(--ease-out);
  }

  .skip:focus {
    transform: none;
  }
</style>
