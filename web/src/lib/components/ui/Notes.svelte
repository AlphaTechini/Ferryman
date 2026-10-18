<script lang="ts">
  /** The quiet right-hand column: a few facts about the crossing, set like marginalia. */
  let { title, items }: { title: string; items: { term: string; text: string }[] } = $props();
</script>

<section class="notes" aria-label={title}>
  <h2 class="eyebrow">{title}</h2>
  <dl>
    {#each items as item (item.term)}
      <div>
        <dt>{item.term}</dt>
        <dd>{item.text}</dd>
      </div>
    {/each}
  </dl>
</section>

<style>
  h2 {
    font-family: var(--font-mono);
    font-weight: 500;
    font-size: var(--fs-2xs);
    letter-spacing: var(--tracking-caps);
    line-height: 1.5;
    padding-bottom: var(--s-3);
    border-bottom: 1px solid var(--line-2);
  }

  dl div {
    display: grid;
    grid-template-columns: minmax(110px, 0.42fr) 1fr;
    gap: var(--s-4);
    padding: var(--s-4) 0;
    border-bottom: 1px solid var(--line);
  }

  dt {
    font-family: var(--font-display);
    font-size: var(--fs-md);
    font-variation-settings: 'opsz' 24;
    line-height: 1.35;
    color: var(--ink);
  }

  dd {
    margin: 0;
    color: var(--ink-2);
    font-size: var(--fs-sm);
    line-height: 1.55;
  }

  @media (max-width: 560px) {
    dl div {
      grid-template-columns: 1fr;
      gap: var(--s-1);
    }
  }
</style>
