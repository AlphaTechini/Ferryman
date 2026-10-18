<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    tone = 'neutral',
    children,
  }: { tone?: 'neutral' | 'lantern' | 'success' | 'danger' | 'river'; children: Snippet } = $props();
</script>

<span class="badge {tone}">{@render children()}</span>

<style>
  .badge {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    height: 22px;
    padding: 0 8px;
    border-radius: var(--r-full);
    font-size: var(--fs-xs);
    font-weight: 500;
    line-height: 1;
    white-space: nowrap;
  }

  .neutral {
    background: var(--bg-sunken);
    color: var(--ink-2);
  }

  .lantern {
    background: var(--lantern-soft);
    color: var(--lantern-ink);
  }

  .success {
    background: var(--success-soft);
    color: var(--success);
  }

  .danger {
    background: var(--danger-soft);
    color: var(--danger);
  }

  .river {
    background: var(--river-soft);
    color: var(--river);
  }
</style>
