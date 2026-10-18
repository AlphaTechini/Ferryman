<script lang="ts">
  import { ROUTES, href, router } from '../../router.svelte';
</script>

<nav class="tabs" aria-label="Ferryman">
  {#each ROUTES as r (r.id)}
    <a
      href={href(r.id)}
      class:active={router.route === r.id}
      aria-current={router.route === r.id ? 'page' : undefined}
      title={r.hint}
    >
      {r.label}
    </a>
  {/each}
</nav>

<style>
  .tabs {
    display: inline-flex;
    gap: 2px;
    padding: 3px;
    border-radius: var(--r-full);
    background: var(--bg-sunken);
    box-shadow: inset 0 0 0 1px var(--line);
  }

  a {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 76px;
    height: 32px;
    padding: 0 var(--s-4);
    border-radius: var(--r-full);
    color: var(--ink-2);
    font-size: var(--fs-sm);
    font-weight: 500;
    text-decoration: none;
    transition: background-color var(--dur) var(--ease-out), color var(--dur) var(--ease-out),
      box-shadow var(--dur) var(--ease-out);
  }

  a:hover {
    color: var(--ink);
  }

  a.active {
    background: var(--bg-raised);
    color: var(--ink);
    box-shadow: 0 1px 2px rgba(0, 0, 0, 0.08), 0 0 0 1px var(--line);
  }

  a:focus-visible {
    border-radius: var(--r-full);
  }

  @media (max-width: 560px) {
    .tabs {
      flex: 1;
    }

    a {
      flex: 1;
      min-width: 0;
      padding: 0 var(--s-2);
    }
  }
</style>
