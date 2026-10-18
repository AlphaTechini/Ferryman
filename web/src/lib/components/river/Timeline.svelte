<script lang="ts" module>
  export type StepState = 'done' | 'active' | 'pending' | 'failed' | 'skipped';
  export type Step = {
    id: string;
    title: string;
    detail?: string;
    state: StepState;
    links?: { label: string; href: string }[];
  };
</script>

<script lang="ts">
  import Icon from '../ui/Icon.svelte';
  import ExternalLink from '../ui/ExternalLink.svelte';

  let { steps, label }: { steps: Step[]; label: string } = $props();
</script>

<ol class="timeline" aria-label={label}>
  {#each steps as step, i (step.id)}
    <li class="step {step.state}" aria-current={step.state === 'active' ? 'step' : undefined}>
      <span class="rail" aria-hidden="true">
        <span class="node">
          {#if step.state === 'done'}
            <Icon name="check" size={12} />
          {:else if step.state === 'failed'}
            <Icon name="undo" size={12} />
          {/if}
        </span>
        {#if i < steps.length - 1}<span class="line"></span>{/if}
      </span>
      <div class="content">
        <p class="title">
          {step.title}
          <span class="visually-hidden">
            ({step.state === 'done' ? 'complete' : step.state === 'active' ? 'in progress' : step.state === 'failed' ? 'refunded' : 'pending'})
          </span>
        </p>
        {#if step.detail}<p class="detail">{step.detail}</p>{/if}
        {#if step.links?.length}
          <p class="links">
            {#each step.links as link (link.href)}
              <ExternalLink href={link.href}>{link.label}</ExternalLink>
            {/each}
          </p>
        {/if}
      </div>
    </li>
  {/each}
</ol>

<style>
  .timeline {
    display: flex;
    flex-direction: column;
  }

  .step {
    display: grid;
    grid-template-columns: 22px 1fr;
    gap: var(--s-4);
  }

  .rail {
    display: flex;
    flex-direction: column;
    align-items: center;
  }

  .node {
    display: grid;
    place-items: center;
    flex: none;
    width: 22px;
    height: 22px;
    margin-top: 1px;
    border-radius: var(--r-full);
    border: 1.5px solid var(--line-strong);
    background: var(--bg-raised);
    color: var(--ink-inverse);
    transition: background-color var(--dur-slow) var(--ease-out), border-color var(--dur-slow) var(--ease-out);
  }

  .line {
    flex: 1;
    width: 1.5px;
    min-height: 16px;
    margin: 4px 0;
    background: repeating-linear-gradient(180deg, var(--line-2) 0 4px, transparent 4px 8px);
  }

  .done .line {
    background: var(--ink);
    opacity: 0.6;
  }

  .done .node {
    background: var(--ink);
    border-color: var(--ink);
  }

  .active .node {
    border-color: var(--lantern);
    background: var(--bg-raised);
    box-shadow: 0 0 0 4px var(--lantern-soft);
  }

  .active .node::after {
    content: '';
    width: 8px;
    height: 8px;
    border-radius: var(--r-full);
    background: var(--lantern);
    animation: pulse 1.8s var(--ease-in-out) infinite;
  }

  .failed .node {
    background: var(--lantern);
    border-color: var(--lantern);
    color: var(--on-lantern);
  }

  .skipped {
    opacity: 0.5;
  }

  .skipped .title {
    text-decoration: line-through;
    text-decoration-color: var(--line-strong);
  }

  .content {
    min-width: 0;
    padding-bottom: var(--s-5);
  }

  .step:last-child .content {
    padding-bottom: 0;
  }

  .title {
    font-weight: 500;
    line-height: 24px;
    color: var(--ink);
  }

  .pending .title {
    color: var(--ink-3);
    font-weight: 400;
  }

  .detail {
    margin-top: 2px;
    font-size: var(--fs-sm);
    color: var(--ink-2);
    line-height: var(--lh-snug);
  }

  .links {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s-1) var(--s-4);
    margin-top: var(--s-2);
  }

  @keyframes pulse {
    0%,
    100% {
      transform: scale(1);
      opacity: 1;
    }
    50% {
      transform: scale(0.6);
      opacity: 0.55;
    }
  }
</style>
