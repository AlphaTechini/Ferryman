<script lang="ts">
  const id = 'demo-note';
</script>

<button type="button" class="demo" aria-describedby={id}>
  <span class="dot" aria-hidden="true"></span>Demo data
  <span {id} role="tooltip" class="note">
    No keeper API is configured (VITE_API_URL), so wallets, quotes and crossings are simulated. The demo clock runs
    fast: a 25 minute exit takes about 40 seconds.
  </span>
</button>

<style>
  .demo {
    position: relative;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 24px;
    padding: 0 9px;
    border: 1px dashed var(--line-strong);
    border-radius: var(--r-full);
    font-family: var(--font-mono);
    font-size: var(--fs-2xs);
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--ink-2);
    white-space: nowrap;
    cursor: help;
  }

  .dot {
    width: 6px;
    height: 6px;
    border-radius: var(--r-full);
    background: var(--lantern);
  }

  .note {
    position: absolute;
    z-index: var(--z-popover);
    top: calc(100% + 8px);
    left: 0;
    width: min(280px, calc(100vw - 32px));
    padding: var(--s-3);
    border-radius: var(--r-md);
    background: var(--bg-inverse);
    color: var(--ink-inverse);
    font-family: var(--font-sans);
    font-size: var(--fs-xs);
    letter-spacing: 0;
    line-height: 1.5;
    text-transform: none;
    white-space: normal;
    box-shadow: var(--shadow-pop);
    opacity: 0;
    pointer-events: none;
    transform: translateY(-4px);
    transition: opacity var(--dur) var(--ease-out), transform var(--dur) var(--ease-out);
  }

  @media (max-width: 560px) {
    .note {
      left: 50%;
      width: 260px;
      transform: translate(-50%, -4px);
    }

    .demo:hover .note.note,
    .demo:focus .note.note {
      transform: translate(-50%, 0);
    }
  }

  .demo:hover .note,
  .demo:focus-visible .note,
  .demo:focus .note {
    opacity: 1;
    transform: none;
  }
</style>
