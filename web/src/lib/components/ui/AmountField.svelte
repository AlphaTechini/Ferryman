<script lang="ts">
  import type { Snippet } from 'svelte';
  import { formatHype } from '../../format';

  let {
    id,
    label,
    value = $bindable(''),
    balance,
    onmax,
    invalid = false,
    side,
    foot,
  }: {
    id: string;
    label: string;
    value: string;
    balance: bigint | null;
    onmax?: () => void;
    invalid?: boolean;
    side: Snippet;
    foot?: Snippet;
  } = $props();

  /** Keep only digits and one decimal point; accept comma as decimal separator. */
  function sanitize(e: Event & { currentTarget: HTMLInputElement }) {
    const raw = e.currentTarget.value.replace(',', '.').replace(/[^\d.]/g, '');
    const [int, ...rest] = raw.split('.');
    value = rest.length ? `${int}.${rest.join('')}` : int;
    e.currentTarget.value = value;
  }
</script>

<div class="field" class:invalid>
  <div class="top">
    <label for={id}>{label}</label>
    {#if balance !== null}
      <span class="balance">
        <span class="num">Balance {formatHype(balance)}</span>
        {#if onmax && balance > 0n}<button type="button" class="max" onclick={onmax}>Max</button>{/if}
      </span>
    {/if}
  </div>
  <div class="row">
    <input
      {id}
      class="num"
      type="text"
      inputmode="decimal"
      autocomplete="off"
      spellcheck="false"
      placeholder="0"
      {value}
      oninput={sanitize}
      aria-invalid={invalid || undefined}
    />
    {@render side()}
  </div>
  {#if foot}<div class="foot">{@render foot()}</div>{/if}
</div>

<style>
  .field {
    padding: var(--s-4) var(--s-5) var(--s-4);
    border-radius: var(--r-lg);
    background: var(--bg-sunken);
    border: 1px solid transparent;
    transition: border-color var(--dur) var(--ease-out), background-color var(--dur) var(--ease-out);
  }

  .field:focus-within {
    border-color: var(--line-strong);
    background: color-mix(in srgb, var(--bg-sunken) 70%, var(--bg-raised));
  }

  .field.invalid {
    border-color: color-mix(in srgb, var(--danger) 55%, transparent);
  }

  .top {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: var(--s-3);
    min-height: 22px;
    font-size: var(--fs-sm);
  }

  label {
    color: var(--ink-2);
  }

  .balance {
    display: inline-flex;
    align-items: center;
    gap: var(--s-2);
    color: var(--ink-3);
    font-size: var(--fs-xs);
  }

  .max {
    height: 22px;
    padding: 0 8px;
    border-radius: var(--r-full);
    background: var(--bg-raised);
    border: 1px solid var(--line-2);
    color: var(--ink);
    font-size: var(--fs-2xs);
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
  }

  .max:hover {
    border-color: var(--line-strong);
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--s-3);
    margin-top: var(--s-2);
  }

  input {
    flex: 1;
    min-width: 0;
    padding: 0;
    border: 0;
    background: none;
    font-family: var(--font-display);
    font-size: 2.5rem;
    font-weight: 400;
    font-variation-settings: 'opsz' 60;
    letter-spacing: -0.02em;
    line-height: 1.15;
    color: var(--ink);
  }

  input::placeholder {
    color: var(--ink-3);
    opacity: 0.6;
  }

  input:focus-visible {
    outline: none;
  }

  .foot {
    margin-top: var(--s-2);
    font-size: var(--fs-xs);
    color: var(--ink-3);
  }

  @media (max-width: 560px) {
    .field {
      padding: var(--s-3) var(--s-4);
    }

    input {
      font-size: 2rem;
    }
  }
</style>
