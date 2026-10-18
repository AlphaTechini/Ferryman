<script lang="ts">
  import Icon from './Icon.svelte';

  let { text, label = 'More info' }: { text: string; label?: string } = $props();
  const id = `tip-${Math.random().toString(36).slice(2, 8)}`;
  let open = $state(false);
</script>

<span class="tip" class:open>
  <button
    type="button"
    aria-label={label}
    aria-describedby={id}
    onclick={() => (open = !open)}
    onblur={() => (open = false)}
    onkeydown={(e) => e.key === 'Escape' && (open = false)}
  >
    <Icon name="info" size={15} />
  </button>
  <span {id} role="tooltip" class="bubble">{text}</span>
</span>

<style>
  .tip {
    position: relative;
    display: inline-flex;
    vertical-align: middle;
  }

  button {
    display: grid;
    place-items: center;
    width: 22px;
    height: 22px;
    border-radius: var(--r-full);
    color: var(--ink-3);
  }

  button:hover {
    color: var(--ink);
  }

  .bubble {
    position: absolute;
    z-index: var(--z-popover);
    bottom: calc(100% + 8px);
    left: 50%;
    width: max-content;
    max-width: min(280px, 80vw);
    padding: var(--s-2) var(--s-3);
    border-radius: var(--r-sm);
    background: var(--bg-inverse);
    color: var(--ink-inverse);
    font-size: var(--fs-xs);
    font-weight: 400;
    line-height: 1.45;
    letter-spacing: 0;
    text-transform: none;
    text-align: left;
    white-space: normal;
    box-shadow: var(--shadow-pop);
    opacity: 0;
    pointer-events: none;
    transform: translate(-50%, 4px);
    transition: opacity var(--dur) var(--ease-out), transform var(--dur) var(--ease-out);
  }

  .tip:hover .bubble,
  .tip:focus-within .bubble,
  .tip.open .bubble {
    opacity: 1;
    transform: translate(-50%, 0);
  }

  @media (max-width: 560px) {
    .bubble {
      left: auto;
      right: -8px;
      transform: translateY(4px);
    }

    .tip:hover .bubble,
    .tip:focus-within .bubble,
    .tip.open .bubble {
      transform: none;
    }
  }
</style>
