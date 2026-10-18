<script lang="ts">
  import { ferry } from '../../services';
  import type { RescueStats } from '../../services/types';
  import { errorMessage } from '../../toasts.svelte';
  import Lookup from './Lookup.svelte';
  import RecentRescues from './RecentRescues.svelte';
  import RescueHero from './RescueHero.svelte';

  let stats = $state<RescueStats | null>(null);
  let error = $state<string | null>(null);

  $effect(() => {
    let stopped = false;
    const load = async () => {
      try {
        const next = await ferry.getRescueStats();
        if (!stopped) {
          stats = next;
          error = null;
        }
      } catch (err) {
        if (!stopped) error = errorMessage(err);
      }
    };
    load();
    const id = setInterval(load, 5000);
    return () => {
      stopped = true;
      clearInterval(id);
    };
  });
</script>

<div class="page">
  <RescueHero {stats} {error} />
  <div class="grid">
    <Lookup />
    <RecentRescues items={stats?.recent ?? (error ? [] : null)} />
  </div>
</div>

<style>
  .page {
    max-width: 1200px;
    margin: 0 auto;
    padding: var(--s-12) var(--gutter) 0;
  }

  .grid {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 400px);
    gap: var(--s-12);
    align-items: start;
    margin-top: var(--s-12);
  }

  @media (max-width: 960px) {
    .grid {
      grid-template-columns: minmax(0, 1fr);
      gap: var(--s-10);
    }
  }

  @media (max-width: 560px) {
    .page {
      padding-top: var(--s-8);
    }

    .grid {
      margin-top: var(--s-8);
    }
  }
</style>
