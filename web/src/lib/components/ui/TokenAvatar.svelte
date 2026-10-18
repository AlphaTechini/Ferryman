<script lang="ts">
  import { hashString } from '../../util/hash';

  let { symbol, address, size = 36 }: { symbol: string; address: string; size?: number } = $props();

  /** A quiet, address-derived tint so look-alike names still look different. */
  const hue = $derived(hashString(address.toLowerCase()) % 360);
</script>

<span class="avatar" style:--size="{size}px" style:--hue={hue} aria-hidden="true">
  {symbol.slice(0, 1)}
</span>

<style>
  .avatar {
    display: grid;
    place-items: center;
    flex: none;
    width: var(--size);
    height: var(--size);
    border-radius: var(--r-full);
    background: oklch(0.9 0.04 var(--hue));
    color: oklch(0.36 0.07 var(--hue));
    box-shadow: inset 0 0 0 1px oklch(0.36 0.07 var(--hue) / 0.16);
    font-family: var(--font-display);
    font-size: calc(var(--size) * 0.5);
    font-weight: 500;
    line-height: 1;
  }

  :global([data-theme='dark']) .avatar {
    background: oklch(0.3 0.05 var(--hue));
    color: oklch(0.88 0.06 var(--hue));
    box-shadow: inset 0 0 0 1px oklch(0.88 0.06 var(--hue) / 0.14);
  }
</style>
