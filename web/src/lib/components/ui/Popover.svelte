<script lang="ts">
  import type { Snippet } from 'svelte';
  import { fly } from 'svelte/transition';
  import { prefersReducedMotion } from 'svelte/motion';

  /** A trigger plus an anchored panel. Closes on Escape, outside click, and focus leaving. */
  let {
    open = $bindable(false),
    align = 'end',
    trigger,
    children,
  }: {
    open: boolean;
    align?: 'start' | 'end';
    trigger: Snippet<[{ toggle: () => void; expanded: boolean }]>;
    children: Snippet;
  } = $props();

  let root = $state<HTMLElement | undefined>();

  function onDocClick(e: MouseEvent) {
    if (open && root && !root.contains(e.target as Node)) open = false;
  }

  function onKey(e: KeyboardEvent) {
    if (open && e.key === 'Escape') {
      open = false;
      root?.querySelector<HTMLElement>('button')?.focus();
    }
  }

  function onFocusOut(e: FocusEvent) {
    if (root && !root.contains(e.relatedTarget as Node)) open = false;
  }
</script>

<svelte:document onclick={onDocClick} onkeydown={onKey} />

<div class="popover" bind:this={root} onfocusout={onFocusOut}>
  {@render trigger({ toggle: () => (open = !open), expanded: open })}
  {#if open}
    <div class="panel {align}" transition:fly={{ y: prefersReducedMotion.current ? 0 : -6, duration: 180 }}>
      {@render children()}
    </div>
  {/if}
</div>

<style>
  .popover {
    position: relative;
  }

  .panel {
    position: absolute;
    z-index: var(--z-popover);
    top: calc(100% + 8px);
    min-width: 240px;
    padding: var(--s-2);
    border: 1px solid var(--line-2);
    border-radius: var(--r-lg);
    background: var(--bg-raised);
    box-shadow: var(--shadow-pop);
  }

  .end {
    right: 0;
  }

  .start {
    left: 0;
  }
</style>
