<script lang="ts">
  import type { Snippet } from 'svelte';

  /** Editorial page frame shared by the three screens: headline, river, then work + notes. */
  let {
    eyebrow,
    title,
    lede,
    river,
    main,
    aside,
    after,
  }: {
    eyebrow: Snippet;
    title: string;
    lede: string;
    river?: Snippet;
    main: Snippet;
    aside?: Snippet;
    after?: Snippet;
  } = $props();
</script>

<div class="page">
  <header class="head">
    <div class="titles">
      <p class="eyebrow">{@render eyebrow()}</p>
      <h1>{title}</h1>
    </div>
    <p class="lede">{lede}</p>
  </header>

  {#if river}<div class="river">{@render river()}</div>{/if}

  <div class="body" class:solo={!aside}>
    <div class="main">{@render main()}</div>
    {#if aside}<aside class="aside">{@render aside()}</aside>{/if}
  </div>

  {#if after}<div class="after">{@render after()}</div>{/if}
</div>

<style>
  .page {
    max-width: 1200px;
    margin: 0 auto;
    padding: var(--s-12) var(--gutter) 0;
  }

  .head {
    display: grid;
    grid-template-columns: minmax(0, 1.35fr) minmax(0, 1fr);
    align-items: end;
    gap: var(--s-6) var(--s-12);
  }

  .titles {
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
  }

  .eyebrow {
    display: inline-flex;
    align-items: center;
    gap: var(--s-2);
  }

  h1 {
    font-size: var(--fs-2xl);
    max-width: 20ch;
  }

  .lede {
    max-width: 44ch;
    padding-bottom: 6px;
    color: var(--ink-2);
    font-size: var(--fs-md);
    line-height: 1.5;
  }

  .river {
    margin-top: var(--s-10);
  }

  .body {
    display: grid;
    grid-template-columns: minmax(0, 500px) minmax(0, 1fr);
    gap: var(--s-12);
    align-items: start;
    margin-top: var(--s-8);
  }

  .body.solo {
    grid-template-columns: minmax(0, 1fr);
  }

  .main {
    min-width: 0;
  }

  .aside {
    min-width: 0;
    padding-top: var(--s-2);
  }

  .after {
    margin-top: var(--s-16);
  }

  @media (max-width: 960px) {
    .head {
      grid-template-columns: 1fr;
      gap: var(--s-4);
    }

    .body {
      grid-template-columns: minmax(0, 1fr);
      gap: var(--s-10);
    }

    .main {
      max-width: 560px;
    }
  }

  @media (max-width: 560px) {
    .page {
      padding-top: var(--s-8);
    }

    .lede {
      font-size: var(--fs-base);
    }

    .river {
      margin-top: var(--s-6);
    }

    .body {
      margin-top: var(--s-6);
    }

    .after {
      margin-top: var(--s-12);
    }
  }
</style>
