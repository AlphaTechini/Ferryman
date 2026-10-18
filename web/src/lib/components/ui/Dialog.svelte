<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';

  let {
    open = $bindable(false),
    title,
    description,
    wide = false,
    children,
  }: { open: boolean; title: string; description?: string; wide?: boolean; children: Snippet } = $props();

  let dialog = $state<HTMLDialogElement | undefined>();
  const id = `dlg-${Math.random().toString(36).slice(2, 8)}`;

  $effect(() => {
    if (!dialog) return;
    if (open && !dialog.open) dialog.showModal();
    if (!open && dialog.open) dialog.close();
  });

  function onBackdrop(e: MouseEvent) {
    if (e.target === dialog) open = false;
  }
</script>

<dialog
  bind:this={dialog}
  class:wide
  aria-labelledby="{id}-title"
  aria-describedby={description ? `${id}-desc` : undefined}
  onclose={() => (open = false)}
  onclick={onBackdrop}
>
  {#if open}
    <div class="panel">
      <header>
        <div>
          <h2 id="{id}-title">{title}</h2>
          {#if description}<p id="{id}-desc">{description}</p>{/if}
        </div>
        <button class="close" onclick={() => (open = false)} aria-label="Close">
          <Icon name="close" size={18} />
        </button>
      </header>
      <div class="body">
        {@render children()}
      </div>
    </div>
  {/if}
</dialog>

<style>
  dialog {
    width: min(460px, calc(100vw - 32px));
    max-height: min(720px, calc(100dvh - 64px));
    padding: 0;
    border: 1px solid var(--line-2);
    border-radius: var(--r-xl);
    background: var(--bg-raised);
    color: var(--ink);
    box-shadow: var(--shadow-pop);
    overflow: hidden;
  }

  dialog.wide {
    width: min(720px, calc(100vw - 32px));
  }

  dialog[open] {
    display: flex;
    flex-direction: column;
    animation: rise var(--dur-slow) var(--ease-out);
  }

  dialog::backdrop {
    background: var(--scrim);
    backdrop-filter: blur(2px);
    animation: fade var(--dur) var(--ease-out);
  }

  .panel {
    display: flex;
    flex-direction: column;
    min-height: 0;
    flex: 1;
  }

  header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--s-4);
    padding: var(--s-6) var(--s-6) var(--s-4);
  }

  h2 {
    font-size: var(--fs-xl);
  }

  header p {
    margin-top: var(--s-2);
    color: var(--ink-2);
    font-size: var(--fs-sm);
  }

  .close {
    display: grid;
    place-items: center;
    width: 36px;
    height: 36px;
    margin: -6px -8px 0 0;
    border-radius: var(--r-full);
    color: var(--ink-2);
    transition: background-color var(--dur) var(--ease-out), color var(--dur) var(--ease-out);
  }

  .close:hover {
    background: var(--bg-hover);
    color: var(--ink);
  }

  .body {
    min-height: 0;
    overflow-y: auto;
    padding: 0 var(--s-6) var(--s-6);
  }

  @keyframes rise {
    from {
      opacity: 0;
      transform: translateY(12px) scale(0.985);
    }
  }

  @keyframes fade {
    from {
      opacity: 0;
    }
  }

  /* Bottom sheet on phones. */
  @media (max-width: 560px) {
    dialog,
    dialog.wide {
      width: 100vw;
      max-width: 100vw;
      max-height: 88dvh;
      margin: auto 0 0;
      border-radius: var(--r-xl) var(--r-xl) 0 0;
      border-bottom: 0;
    }

    dialog[open] {
      animation: sheet var(--dur-slow) var(--ease-out);
    }

    header {
      padding: var(--s-5) var(--s-4) var(--s-3);
    }

    header::before {
      content: '';
      position: absolute;
      top: 8px;
      left: 50%;
      width: 36px;
      height: 4px;
      margin-left: -18px;
      border-radius: var(--r-full);
      background: var(--line-2);
    }

    .body {
      padding: 0 var(--s-4) calc(var(--s-6) + env(safe-area-inset-bottom));
    }
  }

  @keyframes sheet {
    from {
      transform: translateY(40%);
      opacity: 0;
    }
  }
</style>
