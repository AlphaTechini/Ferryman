<script lang="ts">
  let { size = 16 }: { size?: number } = $props();
</script>

<svg class="spinner" width={size} height={size} viewBox="0 0 24 24" fill="none" aria-hidden="true">
  <circle cx="12" cy="12" r="9" stroke="currentColor" stroke-opacity="0.2" stroke-width="2" />
  <path d="M21 12a9 9 0 0 0-9-9" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
</svg>

<style>
  .spinner {
    flex: none;
    animation: spin 0.9s linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .spinner {
      animation-duration: 2.4s !important;
      animation-iteration-count: infinite !important;
    }
  }
</style>
