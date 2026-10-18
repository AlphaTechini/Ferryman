<script lang="ts">
  import { Tween, prefersReducedMotion } from 'svelte/motion';
  import { cubicOut } from 'svelte/easing';

  /** Animates between numeric values; `format` renders the in-between frames. */
  let { value, format }: { value: number; format: (n: number) => string } = $props();

  const tween = new Tween(0, { duration: 1100, easing: cubicOut });

  $effect(() => {
    tween.set(value, { duration: prefersReducedMotion.current ? 0 : 1100 });
  });
</script>

<span class="num" aria-hidden="true">{format(tween.current)}</span><span class="visually-hidden">{format(value)}</span>
