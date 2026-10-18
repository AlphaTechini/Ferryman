<script lang="ts">
  import { app } from '../../app.svelte';
  import ExternalLink from '../ui/ExternalLink.svelte';
  import Mark from './Mark.svelte';
</script>

<footer class="footer">
  <div class="inner">
    <div class="about">
      <Mark size={22} />
      <p>
        Ferryman carries HYPE between HyperEVM and Elysium: one signature each way, a keeper that claims for you, and
        stranded withdrawals returned to their owners.
      </p>
    </div>
    <nav class="links" aria-label="Footer">
      <button onclick={() => (app.howItWorksOpen = true)}>How it works</button>
      <ExternalLink href="https://github.com/">GitHub</ExternalLink>
      <ExternalLink href="https://github.com/">Docs</ExternalLink>
      <ExternalLink href="https://x.com/">X</ExternalLink>
    </nav>
    <p class="fine">
      <span>Testnet. Contracts are immutable with no admin keys; funds can only reach the address you chose.</span>
      <span>Built for the Ascend Elysium Builder Competition.</span>
    </p>
  </div>
</footer>

<style>
  .footer {
    margin-top: var(--s-20);
    border-top: 1px solid var(--line);
  }

  .inner {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: var(--s-6) var(--s-8);
    max-width: 1200px;
    margin: 0 auto;
    padding: var(--s-10) var(--gutter) var(--s-12);
  }

  .about {
    display: flex;
    gap: var(--s-3);
    max-width: 520px;
    color: var(--ink-2);
    font-size: var(--fs-sm);
  }

  .about :global(.mark) {
    margin-top: 1px;
    color: var(--ink);
  }

  .links {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-start;
    gap: var(--s-2) var(--s-5);
  }

  .links button {
    color: var(--ink-2);
    font-size: var(--fs-sm);
  }

  .links button:hover {
    color: var(--ink);
  }

  .fine {
    grid-column: 1 / -1;
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    gap: var(--s-2) var(--s-6);
    padding-top: var(--s-5);
    border-top: 1px solid var(--line);
    color: var(--ink-3);
    font-size: var(--fs-xs);
  }

  @media (max-width: 720px) {
    .inner {
      grid-template-columns: 1fr;
    }
  }
</style>
