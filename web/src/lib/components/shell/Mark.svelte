<script lang="ts">
  /** The obol: Charon's fare, a coin carrying a ferry on the river line, lantern lit. */
  let { size = 28 }: { size?: number } = $props();
</script>

<svg class="mark" width={size} height={size} viewBox="0 0 32 32" fill="none" aria-hidden="true">
  <circle cx="16" cy="16" r="14.6" stroke="currentColor" stroke-width="1.5" />
  <path d="M5.2 21.4c2.1 0 2.1-1.1 4.2-1.1s2.1 1.1 4.2 1.1 2.1-1.1 4.2-1.1 2.1 1.1 4.2 1.1 2.1-1.1 4.2-1.1 1.6.7 2.3 1" stroke="currentColor" stroke-width="1.1" stroke-linecap="round" opacity="0.5" />
  <path d="M8.6 16.6h14.8l-2.3 2.6a1 1 0 0 1-.75.34h-8.7a1 1 0 0 1-.75-.34z" fill="currentColor" />
  <path d="M13.2 16.6 18.2 7.4h2.6v1" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round" />
  <rect x="19.6" y="9.2" width="2.4" height="3.2" rx="0.8" fill="var(--lantern)" />
</svg>

<style>
  .mark {
    flex: none;
  }
</style>
