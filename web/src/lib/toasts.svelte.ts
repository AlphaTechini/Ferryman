export type ToastTone = 'success' | 'error' | 'info';

export type Toast = {
  id: number;
  tone: ToastTone;
  title: string;
  body?: string;
  link?: { href: string; label: string };
};

let seq = 0;

class Toasts {
  items: Toast[] = $state([]);

  push(toast: Omit<Toast, 'id'>, ttl = toast.tone === 'error' ? 8000 : 5000) {
    const id = ++seq;
    this.items = [...this.items, { ...toast, id }].slice(-4);
    setTimeout(() => this.dismiss(id), ttl);
    return id;
  }

  dismiss(id: number) {
    this.items = this.items.filter((t) => t.id !== id);
  }
}

export const toasts = new Toasts();

/** Turn wallet/API failures into a short human sentence. */
export function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    if (err.name === 'UserRejectedError') return 'You cancelled the request in your wallet.';
    const msg = err.message.split('\n')[0];
    return msg.length > 140 ? `${msg.slice(0, 137)}…` : msg;
  }
  return 'Something went wrong. Try again.';
}
