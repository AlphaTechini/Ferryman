<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';

  let { tone = 'info', children }: { tone?: 'info' | 'warn' | 'danger'; children: Snippet } = $props();
</script>

<div class="notice {tone}" role={tone === 'info' ? undefined : 'alert'}>
  <Icon name={tone === 'info' ? 'info' : 'alert'} size={16} />
  <div class="text">{@render children()}</div>
</div>

<style>
  .notice {
    display: flex;
    gap: var(--s-2);
    padding: var(--s-3);
    border-radius: var(--r-md);
    font-size: var(--fs-sm);
    line-height: var(--lh-snug);
  }

  .notice :global(.icon) {
    margin-top: 1px;
  }

  .text {
    min-width: 0;
  }

  .info {
    background: var(--river-soft);
    color: var(--ink-2);
  }

  .info :global(.icon) {
    color: var(--river);
  }

  .warn {
    background: var(--warn-soft);
    color: var(--warn);
  }

  .danger {
    background: var(--danger-soft);
    color: var(--danger);
  }
</style>
