export type Theme = 'light' | 'dark';

const KEY = 'ferryman.theme';
const media = window.matchMedia('(prefers-color-scheme: dark)');

function saved(): Theme | null {
  try {
    const v = localStorage.getItem(KEY);
    return v === 'light' || v === 'dark' ? v : null;
  } catch {
    return null;
  }
}

/** Follows the OS until the user picks a side; their pick is remembered. */
class ThemeState {
  #choice: Theme | null = $state(saved());
  #system: Theme = $state(media.matches ? 'dark' : 'light');
  current: Theme = $derived(this.#choice ?? this.#system);

  constructor() {
    media.addEventListener('change', (e) => (this.#system = e.matches ? 'dark' : 'light'));
    $effect.root(() => {
      $effect(() => {
        document.documentElement.dataset.theme = this.current;
      });
    });
  }

  toggle() {
    this.#choice = this.current === 'dark' ? 'light' : 'dark';
    try {
      localStorage.setItem(KEY, this.#choice);
    } catch {
      /* not persisted; still applies for this visit */
    }
  }
}

export const theme = new ThemeState();
