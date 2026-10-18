<script lang="ts">
  import { formatBps } from '../../format';
  import Icon from '../ui/Icon.svelte';

  let { value = $bindable(150) }: { value: number } = $props();

  const PRESETS = [50, 150, 300];
  const WARN_ABOVE = 200;

  let open = $state(false);
  let custom = $state('');

  function setCustom(e: Event & { currentTarget: HTMLInputElement }) {
    custom = e.currentTarget.value.replace(/[^\d.]/g, '');
    const pct = Number(custom);
    if (custom && Number.isFinite(pct) && pct > 0 && pct <= 50) value = Math.round(pct * 100);
  }
</script>

<div class="slippage">
  <div class="line">
    <span class="label">Max slippage</span>
    <button
      class="value"
      class:warn={value > WARN_ABOVE}
      onclick={() => (open = !open)}
      aria-expanded={open}
      aria-controls="slippage-options"
    >
      {formatBps(value)}
      <Icon name="sliders" size={14} />
    </button>
  </div>

  {#if open}
    <div id="slippage-options" class="options" role="group" aria-label="Max slippage">
      {#each PRESETS as p (p)}
        <button class:active={value === p && !custom} aria-pressed={value === p && !custom} onclick={() => ((value = p), (custom = ''))}>
          {formatBps(p)}{#if p === 150}<span class="default">default</span>{/if}
        </button>
      {/each}
      <label class="custom" class:active={!!custom}>
        <span class="visually-hidden">Custom slippage, percent</span>
        <input type="text" inputmode="decimal" placeholder="Custom" value={custom} oninput={setCustom} />
        <span aria-hidden="true">%</span>
      </label>
    </div>
  {/if}

  {#if value > WARN_ABOVE}
    <p class="warning" role="status">
      <Icon name="alert" size={14} />
      Above 2% you may get a noticeably worse price. If it moves further, the buy refunds instead.
    </p>
  {/if}
</div>

<style>
  .line {
    display: flex;
    justify-content: space-between;
    align-items: center;
    font-size: var(--fs-sm);
  }

  .label {
    color: var(--ink-2);
  }

  .value {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 26px;
    padding: 0 8px;
    margin-right: -8px;
    border-radius: var(--r-sm);
    font-weight: 500;
    font-variant-numeric: tabular-nums;
  }

  .value :global(.icon) {
    color: var(--ink-3);
  }

  .value:hover {
    background: var(--bg-hover);
  }

  .value.warn {
    color: var(--warn);
  }

  .options {
    display: grid;
    grid-template-columns: repeat(3, 1fr) 1.2fr;
    gap: 6px;
    margin-top: var(--s-2);
  }

  .options button,
  .custom {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 4px;
    height: 34px;
    border-radius: var(--r-sm);
    border: 1px solid var(--line-2);
    font-size: var(--fs-sm);
    font-variant-numeric: tabular-nums;
  }

  .options button:hover {
    border-color: var(--line-strong);
  }

  .options .active {
    border-color: var(--ink);
    box-shadow: inset 0 0 0 0.5px var(--ink);
  }

  .default {
    font-size: var(--fs-2xs);
    color: var(--ink-3);
  }

  .custom {
    padding: 0 var(--s-2);
    color: var(--ink-3);
  }

  .custom:focus-within {
    border-color: var(--ink);
  }

  .custom input {
    width: 100%;
    min-width: 0;
    border: 0;
    background: none;
    color: var(--ink);
    text-align: right;
    font-size: var(--fs-sm);
  }

  .custom input:focus-visible {
    outline: none;
  }

  .warning {
    display: flex;
    gap: 6px;
    margin-top: var(--s-2);
    color: var(--warn);
    font-size: var(--fs-xs);
    line-height: 1.45;
  }

  .warning :global(.icon) {
    margin-top: 1px;
  }

  @media (max-width: 400px) {
    .default {
      display: none;
    }
  }
</style>
