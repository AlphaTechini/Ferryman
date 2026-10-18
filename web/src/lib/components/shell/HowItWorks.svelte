<script lang="ts">
  import { app } from '../../app.svelte';
  import { href } from '../../router.svelte';
  import Dialog from '../ui/Dialog.svelte';

  const flows = [
    {
      route: 'in' as const,
      name: 'Ferry In',
      sign: 'One HyperEVM signature',
      steps: [
        'Pick a token launched on Ascend and an amount of HYPE.',
        'Your HYPE and a buy order cross to Elysium in about a minute.',
        'Ferryman buys the token and delivers it to your address. If the price moved too far, you get your HYPE back on Elysium instead.',
      ],
    },
    {
      route: 'out' as const,
      name: 'Ferry Out',
      sign: 'One Elysium signature',
      steps: [
        'Choose an amount and who receives it on HyperEVM.',
        'The withdrawal matures in about 25 minutes. You can close the tab.',
        'The keeper claims it for a small tip and pays your recipient. No HyperEVM gas, no second visit.',
      ],
    },
    {
      route: 'rescue' as const,
      name: 'Rescue',
      sign: 'Nothing to sign',
      steps: [
        'The keeper finds withdrawals that matured but were never claimed.',
        'It claims them, paying gas from the treasury.',
        'Funds always go to the original owner. Ferryman cannot redirect them.',
      ],
    },
  ];
</script>

<Dialog bind:open={app.howItWorksOpen} title="How Ferryman works" description="Three crossings, one keeper underneath." wide>
  <ol class="flows">
    {#each flows as f, i (f.route)}
      <li>
        <p class="eyebrow">0{i + 1} · {f.sign}</p>
        <h3><a href={href(f.route)} onclick={() => (app.howItWorksOpen = false)}>{f.name}</a></h3>
        <ol class="steps">
          {#each f.steps as s (s)}<li>{s}</li>{/each}
        </ol>
      </li>
    {/each}
  </ol>
  <p class="extra">
    <strong>Optional last step.</strong> Once HYPE lands on HyperEVM, one more signature sends it to your Hyperliquid trading
    balance through Hyperliquid's own system address.
  </p>
</Dialog>

<style>
  .flows {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--s-6);
  }

  .flows > li {
    padding-top: var(--s-4);
    border-top: 1px solid var(--line-2);
  }

  h3 {
    margin: var(--s-2) 0 var(--s-3);
    font-size: var(--fs-xl);
  }

  h3 a {
    text-decoration: none;
  }

  h3 a:hover {
    text-decoration: underline;
    text-decoration-color: var(--line-strong);
  }

  .steps {
    display: grid;
    gap: var(--s-2);
    counter-reset: step;
    font-size: var(--fs-sm);
    color: var(--ink-2);
  }

  .steps li {
    position: relative;
    padding-left: 22px;
    counter-increment: step;
  }

  .steps li::before {
    content: counter(step);
    position: absolute;
    left: 0;
    top: 1px;
    font-family: var(--font-mono);
    font-size: var(--fs-2xs);
    color: var(--lantern-ink);
  }

  .extra {
    margin-top: var(--s-6);
    padding: var(--s-4);
    border-radius: var(--r-md);
    background: var(--bg-sunken);
    font-size: var(--fs-sm);
    color: var(--ink-2);
  }

  .extra strong {
    color: var(--ink);
    font-weight: 500;
  }

  @media (max-width: 640px) {
    .flows {
      grid-template-columns: 1fr;
      gap: var(--s-5);
    }
  }
</style>
