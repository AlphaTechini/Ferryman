<script lang="ts">
  import { MediaQuery } from 'svelte/reactivity';
  import { app } from '../../app.svelte';
  import { isDemo } from '../../services';
  import Brand from './Brand.svelte';
  import DemoBadge from './DemoBadge.svelte';
  import NavTabs from './NavTabs.svelte';
  import NetworkMenu from './NetworkMenu.svelte';
  import ThemeToggle from './ThemeToggle.svelte';
  import WalletButton from './WalletButton.svelte';

  const compact = new MediaQuery('max-width: 860px');
  let scrolled = $state(false);
</script>

<svelte:window onscroll={() => (scrolled = window.scrollY > 4)} />

<header class="header" class:scrolled>
  <div class="inner">
    <div class="start">
      <Brand />
      {#if isDemo}<DemoBadge />{/if}
    </div>
    <div class="nav">
      <NavTabs />
      {#if compact.current}<NetworkMenu />{/if}
    </div>
    <div class="end">
      {#if !compact.current}
        <button class="how" onclick={() => (app.howItWorksOpen = true)}>How it works</button>
        <NetworkMenu />
      {/if}
      <WalletButton />
      <ThemeToggle />
    </div>
  </div>
</header>

<style>
  .header {
    position: sticky;
    top: 0;
    z-index: var(--z-header);
    background: color-mix(in srgb, var(--bg) 86%, transparent);
    backdrop-filter: saturate(1.2) blur(12px);
    -webkit-backdrop-filter: saturate(1.2) blur(12px);
    border-bottom: 1px solid transparent;
    transition: border-color var(--dur) var(--ease-out);
  }

  .header.scrolled {
    border-bottom-color: var(--line);
  }

  .inner {
    display: grid;
    grid-template-columns: 1fr auto 1fr;
    grid-template-areas: 'start nav end';
    align-items: center;
    gap: var(--s-4);
    max-width: 1200px;
    height: 68px;
    margin: 0 auto;
    padding: 0 var(--gutter);
  }

  .start {
    grid-area: start;
    display: flex;
    align-items: center;
    gap: var(--s-3);
    min-width: 0;
  }

  .nav {
    grid-area: nav;
    display: flex;
    align-items: center;
    gap: var(--s-2);
  }

  .end {
    grid-area: end;
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: var(--s-2);
  }

  .how {
    height: 36px;
    padding: 0 var(--s-3);
    border-radius: var(--r-full);
    color: var(--ink-2);
    font-size: var(--fs-sm);
    white-space: nowrap;
  }

  .how:hover {
    color: var(--ink);
    background: var(--bg-hover);
  }

  @media (max-width: 860px) {
    .inner {
      grid-template-columns: 1fr auto;
      grid-template-areas:
        'start end'
        'nav nav';
      height: auto;
      row-gap: var(--s-3);
      padding-top: var(--s-3);
      padding-bottom: var(--s-3);
    }

    .end {
      gap: var(--s-1);
    }
  }
</style>
