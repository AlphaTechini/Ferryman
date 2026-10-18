<script lang="ts">
  import { formatAmount, formatBps, formatDuration, formatHype } from '../../format';
  import type { Quote, Token } from '../../services/types';
  import Skeleton from '../ui/Skeleton.svelte';
  import Tooltip from '../ui/Tooltip.svelte';
  import SlippageControl from './SlippageControl.svelte';

  let {
    quote,
    token,
    loading,
    feeBps,
    slippageBps = $bindable(),
  }: { quote: Quote | null; token: Token; loading: boolean; feeBps: number; slippageBps: number } = $props();

  const gas = $derived(
    quote ? BigInt(quote.retryable.totalValue) - BigInt(quote.amountIn) : 0n,
  );
  const rate = $derived.by(() => {
    if (!quote || BigInt(quote.amountOut) === 0n) return null;
    const net = BigInt(quote.amountIn) - BigInt(quote.fee);
    return (net * 10n ** BigInt(token.decimals)) / BigInt(quote.amountOut);
  });
  const highImpact = $derived((quote?.priceImpactBps ?? 0) > 300);
</script>

<div class="details" class:stale={loading && quote}>
  {#if !quote}
    <dl aria-busy="true" aria-label="Loading quote">
      {#each Array(4) as _, i (i)}
        <div><Skeleton width="34%" height="12px" /><Skeleton width="22%" height="12px" /></div>
      {/each}
    </dl>
  {:else}
    <dl>
      <div>
        <dt>Rate</dt>
        <dd class="num">1 {token.symbol} = {rate === null ? '—' : formatAmount(rate, 18, { max: 8 })} HYPE</dd>
      </div>
      <div>
        <dt>
          Minimum received
          <Tooltip text="If the pool would give you less than this when your order lands on Elysium, Ferryman refunds your HYPE there instead of buying." />
        </dt>
        <dd class="num">{formatAmount(quote.minOut, token.decimals)} {token.symbol}</dd>
      </div>
      <div>
        <dt>Ferryman fee <span class="faint">{formatBps(feeBps)}</span></dt>
        <dd class="num">{formatHype(quote.fee)} HYPE</dd>
      </div>
      <div>
        <dt>Price impact</dt>
        <dd class="num" class:warn={highImpact}>{formatBps(quote.priceImpactBps)}</dd>
      </div>
      <div>
        <dt>
          Crossing gas
          <Tooltip text="Paid on HyperEVM with the ticket: the cost of carrying your order and running the buy on Elysium." />
        </dt>
        <dd class="num">{formatHype(gas)} HYPE</dd>
      </div>
      <div>
        <dt>Route</dt>
        <dd>
          <ol class="route" aria-label="Swap route">
            {#each quote.route as hop, i (hop + i)}
              <li>{hop}</li>
            {/each}
          </ol>
        </dd>
      </div>
      <div>
        <dt>Arrives</dt>
        <dd>{formatDuration(60)} on Elysium</dd>
      </div>
    </dl>
  {/if}
  <div class="slip">
    <SlippageControl bind:value={slippageBps} />
  </div>
</div>

<style>
  .details {
    transition: opacity var(--dur) var(--ease-out);
  }

  .details.stale dl {
    opacity: 0.5;
  }

  dl {
    display: flex;
    flex-direction: column;
    gap: 10px;
    transition: opacity var(--dur) var(--ease-out);
  }

  dl div {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: var(--s-4);
    min-height: 20px;
    font-size: var(--fs-sm);
  }

  dt {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    color: var(--ink-2);
  }

  dd {
    margin: 0;
    text-align: right;
    color: var(--ink);
  }

  dd.warn {
    color: var(--warn);
  }

  .route {
    display: inline-flex;
    align-items: center;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 4px;
    font-size: var(--fs-xs);
    font-weight: 500;
  }

  .route li {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }

  .route li + li::before {
    content: '→';
    color: var(--ink-3);
    font-weight: 400;
  }

  .slip {
    margin-top: var(--s-3);
    padding-top: var(--s-3);
    border-top: 1px solid var(--line);
  }
</style>
