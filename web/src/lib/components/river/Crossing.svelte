<script lang="ts">
  /**
   * The river. HyperEVM is always the west bank and Elysium the east bank, so the map never
   * flips; the ferry travels east for Ferry In and west for Ferry Out. `progress` is 0..1,
   * `stops` marks the stage boundaries along the route.
   */
  type Status = 'idle' | 'moving' | 'arrived' | 'warn';

  let {
    direction,
    progress = 0,
    status = 'idle',
    stops = [],
    label,
  }: { direction: 'in' | 'out'; progress?: number; status?: Status; stops?: number[]; label: string } = $props();

  const clamp = (n: number) => Math.min(1, Math.max(0, n));
  const x = $derived(direction === 'in' ? clamp(progress) : 1 - clamp(progress));
  const fillStart = $derived(direction === 'in' ? 0 : x);
  const fillEnd = $derived(direction === 'in' ? x : 1);
  const at = (p: number) => (direction === 'in' ? p : 1 - p);
</script>

<figure class="crossing {direction} {status}" role="img" aria-label={label}>
  <div class="bank west" class:origin={direction === 'in'} class:dest={direction === 'out'}>
    <span class="name">HyperEVM</span>
    <span class="meta"><span class="long">Hyperliquid&nbsp;·&nbsp;</span>998</span>
  </div>

  <div class="water" aria-hidden="true">
    <span class="wave w1"></span>
    <span class="wave w2"></span>
    <span class="wave w3"></span>

    <div class="route">
      <span class="track"></span>
      <span class="fill" style:left="{fillStart * 100}%" style:width="{(fillEnd - fillStart) * 100}%"></span>
      {#each stops as stop (stop)}
        <span class="stop" class:reached={progress >= stop - 0.001} style:left="{at(stop) * 100}%"></span>
      {/each}
      <span class="ferry" style:left="{x * 100}%">
        <svg viewBox="0 0 44 30" width="58" height="40">
          <path class="pole" d="M17 19.5 26 4.5" />
          <path class="hanger" d="M26 4.5h4.5v2.2" />
          <circle class="glow" cx="30.5" cy="9.6" r="6" />
          <rect class="lantern" x="28.6" y="7" width="3.8" height="5" rx="1.2" />
          <path class="hull" d="M5 19.5h34l-5.2 6.2a2 2 0 0 1-1.5.7H11.7a2 2 0 0 1-1.5-.7z" />
        </svg>
      </span>
    </div>
  </div>

  <div class="bank east" class:origin={direction === 'out'} class:dest={direction === 'in'}>
    <span class="name">Elysium</span>
    <span class="meta"><span class="long">Kinetiq L2&nbsp;·&nbsp;</span>99801</span>
  </div>
</figure>

<style>
  .crossing {
    --water-h: 84px;
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    align-items: stretch;
    margin: 0;
  }

  /* ---------- Banks ---------- */
  .bank {
    position: relative;
    display: flex;
    flex-direction: column;
    justify-content: center;
    gap: 2px;
    padding: 0 var(--s-5);
    min-height: var(--water-h);
  }

  .west {
    padding-left: 0;
    border-right: 1px solid var(--line-2);
  }

  .east {
    padding-right: 0;
    border-left: 1px solid var(--line-2);
    text-align: right;
    align-items: flex-end;
  }

  .name {
    font-family: var(--font-display);
    font-size: var(--fs-lg);
    font-variation-settings: 'opsz' 36;
    letter-spacing: -0.01em;
    line-height: 1.1;
    color: var(--ink);
    transition: color var(--dur-slow) var(--ease-out);
  }

  .meta {
    font-family: var(--font-mono);
    font-size: var(--fs-2xs);
    letter-spacing: 0.02em;
    color: var(--ink-3);
    white-space: nowrap;
  }

  /* The destination bank catches the lantern light on arrival. */
  .bank::after {
    content: '';
    position: absolute;
    top: 50%;
    width: 7px;
    height: 7px;
    margin-top: -3.5px;
    border-radius: var(--r-full);
    background: var(--bg);
    border: 1.5px solid var(--line-strong);
    transition: background-color var(--dur-slow) var(--ease-out), border-color var(--dur-slow) var(--ease-out),
      box-shadow var(--dur-slow) var(--ease-out);
  }

  .west::after {
    right: -4px;
  }

  .east::after {
    left: -4px;
  }

  .bank.origin::after {
    background: var(--ink);
    border-color: var(--ink);
  }

  .arrived .bank.dest::after {
    background: var(--lantern);
    border-color: var(--lantern);
    box-shadow: 0 0 0 4px var(--lantern-soft), 0 0 14px var(--lantern-glow);
  }

  .arrived .bank.dest .name {
    color: var(--lantern-ink);
  }

  /* ---------- Water ---------- */
  .water {
    position: relative;
    height: var(--water-h);
    overflow: hidden;
    background: linear-gradient(180deg, transparent, var(--river-soft) 50%, transparent);
  }

  .wave {
    position: absolute;
    left: 0;
    right: 0;
    height: 12px;
    background: var(--river-wave);
    -webkit-mask: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='120' height='12' viewBox='0 0 120 12'%3E%3Cpath d='M0 6c15-6 15-6 30 0s15 6 30 0 15-6 30 0 15 6 30 0' fill='none' stroke='%23000' stroke-width='1.1'/%3E%3C/svg%3E")
      0 0 / 120px 12px repeat-x;
    mask: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='120' height='12' viewBox='0 0 120 12'%3E%3Cpath d='M0 6c15-6 15-6 30 0s15 6 30 0 15-6 30 0 15 6 30 0' fill='none' stroke='%23000' stroke-width='1.1'/%3E%3C/svg%3E")
      0 0 / 120px 12px repeat-x;
    animation: drift 16s linear infinite;
  }

  .w1 {
    top: 14%;
    opacity: 0.55;
    animation-duration: 22s;
  }

  .w2 {
    top: 66%;
    opacity: 0.9;
    animation-duration: 14s;
    animation-direction: reverse;
  }

  .w3 {
    top: 82%;
    opacity: 0.4;
    -webkit-mask-size: 90px 9px;
    mask-size: 90px 9px;
    animation-duration: 30s;
  }

  @keyframes drift {
    to {
      -webkit-mask-position: -120px 0;
      mask-position: -120px 0;
    }
  }

  /* ---------- Route ---------- */
  .route {
    position: absolute;
    top: 50%;
    left: 28px;
    right: 28px;
    height: 0;
  }

  .track {
    position: absolute;
    inset: -0.75px 0;
    height: 1.5px;
    background: repeating-linear-gradient(90deg, var(--line-strong) 0 5px, transparent 5px 11px);
  }

  .fill {
    position: absolute;
    top: -1px;
    height: 2px;
    border-radius: 2px;
    background: var(--lantern);
    box-shadow: 0 0 10px var(--lantern-glow);
    transition: left var(--dur-crossing) var(--ease-in-out), width var(--dur-crossing) var(--ease-in-out);
  }

  .stop {
    position: absolute;
    top: -3.5px;
    width: 7px;
    height: 7px;
    margin-left: -3.5px;
    border-radius: var(--r-full);
    background: var(--bg);
    border: 1.5px solid var(--line-strong);
    transition: background-color var(--dur-slow) var(--ease-out), border-color var(--dur-slow) var(--ease-out);
  }

  .stop.reached {
    background: var(--lantern);
    border-color: var(--lantern);
  }

  .ferry {
    position: absolute;
    top: -33px;
    width: 58px;
    height: 40px;
    margin-left: -29px;
    transition: left var(--dur-crossing) var(--ease-in-out);
  }

  .out .ferry svg {
    transform: scaleX(-1);
  }

  .ferry svg {
    overflow: visible;
    animation: bob 3.6s var(--ease-in-out) infinite;
  }

  .hull {
    fill: var(--ink);
  }

  .pole,
  .hanger {
    fill: none;
    stroke: var(--ink);
    stroke-width: 1.6;
    stroke-linecap: round;
    stroke-linejoin: round;
  }

  .lantern {
    fill: var(--lantern);
  }

  .glow {
    fill: var(--lantern);
    opacity: 0.45;
    filter: blur(4px);
  }

  .moving .glow {
    animation: flicker 2.4s var(--ease-in-out) infinite;
  }

  .warn .lantern {
    fill: var(--danger);
  }

  .warn .glow {
    fill: var(--danger);
    opacity: 0.25;
  }

  @keyframes bob {
    0%,
    100% {
      transform: translateY(0) rotate(0deg);
    }
    50% {
      transform: translateY(-1.5px) rotate(-1.2deg);
    }
  }

  .out .ferry svg {
    animation-name: bob-out;
  }

  @keyframes bob-out {
    0%,
    100% {
      transform: scaleX(-1) translateY(0) rotate(0deg);
    }
    50% {
      transform: scaleX(-1) translateY(-1.5px) rotate(-1.2deg);
    }
  }

  @keyframes flicker {
    0%,
    100% {
      opacity: 0.45;
    }
    45% {
      opacity: 0.9;
    }
    60% {
      opacity: 0.55;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .wave,
    .ferry svg,
    .moving .glow {
      animation: none !important;
    }
  }

  @media (max-width: 560px) {
    .crossing {
      --water-h: 72px;
    }

    .bank {
      padding: 0 var(--s-3);
    }

    .west {
      padding-left: 0;
    }

    .east {
      padding-right: 0;
    }

    .name {
      font-size: var(--fs-md);
    }

    .long {
      display: none;
    }

    .route {
      left: 22px;
      right: 22px;
    }
  }
</style>
