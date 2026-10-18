<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';

  let { href, children }: { href: string; children: Snippet } = $props();
</script>

<a class="ext" {href} target="_blank" rel="noopener noreferrer">
  {@render children()}<Icon name="arrow-up-right" size={13} />
  <span class="visually-hidden">(opens in a new tab)</span>
</a>

<style>
  .ext {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    color: var(--ink-2);
    font-size: var(--fs-sm);
    text-decoration: none;
    white-space: nowrap;
  }

  .ext:hover {
    color: var(--ink);
  }
</style>
