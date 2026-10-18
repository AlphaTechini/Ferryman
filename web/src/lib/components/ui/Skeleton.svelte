<script lang="ts">
  let { width = '100%', height = '1em', radius = 'var(--r-xs)' }: { width?: string; height?: string; radius?: string } =
    $props();
</script>

<span class="skeleton" style:width style:height style:border-radius={radius} aria-hidden="true"></span>

<style>
  .skeleton {
    display: block;
    background: linear-gradient(90deg, var(--bg-sunken) 0%, var(--bg-hover) 50%, var(--bg-sunken) 100%) 0 0 / 300% 100%;
    background-color: var(--bg-sunken);
    animation: shimmer 1.6s var(--ease-in-out) infinite;
  }

  @keyframes shimmer {
    from {
      background-position: 100% 0;
    }
    to {
      background-position: 0 0;
    }
  }
</style>
