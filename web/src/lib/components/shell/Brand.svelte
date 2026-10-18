<script lang="ts">
  import { href } from '../../router.svelte';
  import Mark from './Mark.svelte';
</script>

<a class="brand" href={href('in')} aria-label="Ferryman, home">
  <Mark size={28} />
  <span class="word">Ferryman</span>
</a>

<style>
  .brand {
    display: inline-flex;
    align-items: center;
    gap: 10px;
    color: var(--ink);
    text-decoration: none;
    border-radius: var(--r-sm);
  }

  .word {
    font-family: var(--font-display);
    font-size: 1.375rem;
    font-weight: 500;
    font-variation-settings: 'opsz' 48;
    letter-spacing: -0.025em;
    line-height: 1;
    padding-bottom: 2px;
  }
</style>
