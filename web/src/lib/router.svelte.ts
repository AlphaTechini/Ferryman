export type Route = 'in' | 'out' | 'rescue';

export const ROUTES: { id: Route; label: string; hint: string }[] = [
  { id: 'in', label: 'In', hint: 'Buy an Elysium token from HyperEVM' },
  { id: 'out', label: 'Out', hint: 'Leave Elysium with nothing to claim' },
  { id: 'rescue', label: 'Rescue', hint: 'Stranded withdrawals, returned' },
];

function parse(hash: string): Route {
  const id = hash.replace(/^#\/?/, '').split(/[?/]/)[0];
  return ROUTES.some((r) => r.id === id) ? (id as Route) : 'in';
}

/** Hash routing: #/in, #/out, #/rescue. */
class Router {
  route: Route = $state(parse(location.hash));

  constructor() {
    window.addEventListener('hashchange', () => {
      this.route = parse(location.hash);
      window.scrollTo({ top: 0 });
    });
  }
}

export const router = new Router();
export const href = (route: Route) => `#/${route}`;
