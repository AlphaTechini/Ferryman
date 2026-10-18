<script lang="ts">
  import { theme } from '../../theme.svelte';
  import Icon from '../ui/Icon.svelte';

  const next = $derived(theme.current === 'dark' ? 'light' : 'dark');
</script>

<button class="toggle" onclick={() => theme.toggle()} aria-label="Switch to {next} theme" title="Switch to {next} theme">
  <Icon name={theme.current === 'dark' ? 'sun' : 'moon'} size={18} />
</button>

<style>
  .toggle {
    display: grid;
    place-items: center;
    width: 40px;
    height: 40px;
    border-radius: var(--r-full);
    color: var(--ink-2);
    transition: background-color var(--dur) var(--ease-out), color var(--dur) var(--ease-out);
  }

  .toggle:hover {
    background: var(--bg-hover);
    color: var(--ink);
  }

  .toggle:focus-visible {
    border-radius: var(--r-full);
  }
</style>
