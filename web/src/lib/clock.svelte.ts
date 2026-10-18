/** One shared 1 Hz clock (unix seconds) for countdowns and relative times. */
class Clock {
  now = $state(Date.now() / 1000);

  constructor() {
    setInterval(() => (this.now = Date.now() / 1000), 1000);
  }
}

export const clock = new Clock();
