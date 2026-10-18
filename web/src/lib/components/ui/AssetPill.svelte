<script lang="ts">
  /** Static asset label for an amount field: HYPE, and which chain it sits on. */
  let { symbol, chain }: { symbol: string; chain: string } = $props();
</script>

<span class="pill">
  <span class="glyph" aria-hidden="true">H</span>
  <span class="text">
    <span class="symbol">{symbol}</span>
    <span class="chain">on {chain}</span>
  </span>
</span>

<style>
  .pill {
    display: inline-flex;
    align-items: center;
    gap: var(--s-2);
    flex: none;
    height: 44px;
    padding: 0 var(--s-4) 0 6px;
    border-radius: var(--r-full);
    background: var(--bg-raised);
    border: 1px solid var(--line);
  }

  .glyph {
    display: grid;
    place-items: center;
    width: 30px;
    height: 30px;
    border-radius: var(--r-full);
    background: var(--river);
    color: var(--bg-raised);
    font-family: var(--font-display);
    font-size: 15px;
    font-weight: 600;
  }

  .text {
    display: flex;
    flex-direction: column;
    line-height: 1.15;
  }

  .symbol {
    font-weight: 600;
    font-size: var(--fs-sm);
  }

  .chain {
    font-size: var(--fs-2xs);
    color: var(--ink-3);
  }
</style>
