<script lang="ts">
  import Icon from './Icon.svelte';

  let { value, label = 'Copy' }: { value: string; label?: string } = $props();
  let copied = $state(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(value);
      copied = true;
      setTimeout(() => (copied = false), 1400);
    } catch {
      copied = false;
    }
  }
</script>

<button class="copy" onclick={copy} aria-label={copied ? 'Copied' : label} title={copied ? 'Copied' : label}>
  <Icon name={copied ? 'check' : 'copy'} size={14} />
</button>

<style>
  .copy {
    display: inline-grid;
    place-items: center;
    width: 24px;
    height: 24px;
    border-radius: var(--r-sm);
    color: var(--ink-3);
    transition: color var(--dur) var(--ease-out), background-color var(--dur) var(--ease-out);
  }

  .copy:hover {
    color: var(--ink);
    background: var(--bg-hover);
  }
</style>
