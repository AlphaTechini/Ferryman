<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLButtonAttributes } from 'svelte/elements';
  import Spinner from './Spinner.svelte';

  type Props = HTMLButtonAttributes & {
    variant?: 'primary' | 'secondary' | 'ghost' | 'quiet';
    size?: 'sm' | 'md' | 'lg';
    full?: boolean;
    loading?: boolean;
    href?: string;
    children: Snippet;
  };

  let {
    variant = 'secondary',
    size = 'md',
    full = false,
    loading = false,
    href,
    disabled,
    type = 'button',
    children,
    ...rest
  }: Props = $props();
</script>

{#if href}
  <a class="btn {variant} {size}" class:full {href} target="_blank" rel="noopener noreferrer">
    {@render children()}
  </a>
{:else}
  <button
    class="btn {variant} {size}"
    class:full
    class:loading
    {type}
    disabled={disabled || loading}
    aria-busy={loading || undefined}
    {...rest}
  >
    {#if loading}<Spinner size={size === 'lg' ? 18 : 15} />{/if}
    {@render children()}
  </button>
{/if}

<style>
  .btn {
    position: relative;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: var(--s-2);
    border-radius: var(--r-md);
    font-weight: 500;
    line-height: 1;
    white-space: nowrap;
    text-decoration: none;
    transition:
      background-color var(--dur) var(--ease-out),
      color var(--dur) var(--ease-out),
      border-color var(--dur) var(--ease-out),
      transform var(--dur-fast) var(--ease-out),
      opacity var(--dur) var(--ease-out);
    -webkit-tap-highlight-color: transparent;
  }

  .btn:active:not(:disabled) {
    transform: translateY(1px) scale(0.995);
  }

  .full {
    width: 100%;
  }

  .sm {
    height: 32px;
    padding: 0 var(--s-3);
    font-size: var(--fs-sm);
    border-radius: var(--r-sm);
  }

  .md {
    height: 40px;
    padding: 0 var(--s-4);
    font-size: var(--fs-sm);
  }

  .lg {
    height: 56px;
    padding: 0 var(--s-6);
    font-size: var(--fs-md);
    border-radius: var(--r-lg);
    letter-spacing: -0.005em;
  }

  .primary {
    background: var(--bg-inverse);
    color: var(--ink-inverse);
  }

  .primary:hover:not(:disabled) {
    background: color-mix(in srgb, var(--bg-inverse) 88%, var(--lantern));
  }

  .secondary {
    background: var(--bg-raised);
    color: var(--ink);
    border: 1px solid var(--line-2);
  }

  .secondary:hover:not(:disabled) {
    border-color: var(--line-strong);
    background: color-mix(in srgb, var(--bg-raised) 92%, var(--ink));
  }

  .ghost {
    color: var(--ink);
    border: 1px solid transparent;
  }

  .ghost:hover:not(:disabled) {
    background: var(--bg-hover);
  }

  .quiet {
    height: auto;
    padding: 0;
    color: var(--lantern-ink);
    font-weight: 500;
  }

  .quiet:hover:not(:disabled) {
    text-decoration: underline;
    text-underline-offset: 0.2em;
  }

  .btn:disabled {
    opacity: 0.45;
  }

  .primary:disabled {
    background: var(--bg-sunken);
    color: var(--ink-2);
    opacity: 1;
  }

  .btn.loading:disabled {
    opacity: 0.85;
    cursor: progress;
  }
</style>
