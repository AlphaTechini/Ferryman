<script lang="ts">
  import { fly } from 'svelte/transition';
  import { flip } from 'svelte/animate';
  import { prefersReducedMotion } from 'svelte/motion';
  import { toasts } from '../../toasts.svelte';
  import Icon from './Icon.svelte';

  const icon = { success: 'check', error: 'alert', info: 'info' } as const;
</script>

<section class="toaster" aria-label="Notifications">
  <ol aria-live="polite" aria-relevant="additions">
    {#each toasts.items as t (t.id)}
      <li
        class="toast {t.tone}"
        role={t.tone === 'error' ? 'alert' : 'status'}
        in:fly={{ y: prefersReducedMotion.current ? 0 : 16, duration: 280 }}
        out:fly={{ x: prefersReducedMotion.current ? 0 : 24, duration: 200 }}
        animate:flip={{ duration: 240 }}
      >
        <span class="mark"><Icon name={icon[t.tone]} size={15} /></span>
        <div class="copy">
          <p class="title">{t.title}</p>
          {#if t.body}<p class="body">{t.body}</p>{/if}
          {#if t.link}
            <a href={t.link.href} target="_blank" rel="noopener noreferrer">{t.link.label}</a>
          {/if}
        </div>
        <button class="dismiss" onclick={() => toasts.dismiss(t.id)} aria-label="Dismiss notification">
          <Icon name="close" size={14} />
        </button>
      </li>
    {/each}
  </ol>
</section>

<style>
  .toaster {
    position: fixed;
    z-index: var(--z-toast);
    right: var(--s-6);
    bottom: var(--s-6);
    width: min(380px, calc(100vw - 32px));
    pointer-events: none;
  }

  ol {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }

  .toast {
    pointer-events: auto;
    display: grid;
    grid-template-columns: auto 1fr auto;
    gap: var(--s-3);
    align-items: start;
    padding: var(--s-3) var(--s-3) var(--s-3) var(--s-4);
    border-radius: var(--r-lg);
    background: var(--bg-inverse);
    color: var(--ink-inverse);
    box-shadow: var(--shadow-pop);
  }

  .mark {
    display: grid;
    place-items: center;
    width: 22px;
    height: 22px;
    margin-top: 1px;
    border-radius: var(--r-full);
    background: color-mix(in srgb, var(--ink-inverse) 12%, transparent);
  }

  .success .mark {
    background: var(--lantern);
    color: var(--on-lantern);
  }

  .error .mark {
    background: var(--danger);
    color: #fff;
  }

  .title {
    font-weight: 500;
    font-size: var(--fs-sm);
    line-height: 1.5;
  }

  .body {
    margin-top: 2px;
    font-size: var(--fs-sm);
    line-height: 1.45;
    opacity: 0.72;
  }

  a {
    display: inline-block;
    margin-top: var(--s-1);
    font-size: var(--fs-sm);
    color: inherit;
    text-decoration-color: color-mix(in srgb, var(--ink-inverse) 40%, transparent);
  }

  .dismiss {
    display: grid;
    place-items: center;
    width: 26px;
    height: 26px;
    border-radius: var(--r-full);
    opacity: 0.6;
  }

  .dismiss:hover {
    opacity: 1;
    background: color-mix(in srgb, var(--ink-inverse) 10%, transparent);
  }

  .dismiss:focus-visible,
  a:focus-visible {
    outline-color: var(--lantern);
  }

  @media (max-width: 560px) {
    .toaster {
      right: 16px;
      left: 16px;
      bottom: calc(16px + env(safe-area-inset-bottom));
      width: auto;
    }
  }
</style>
