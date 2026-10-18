import { ferry, wallet } from './services';
import { ELYSIUM_CHAIN_ID, HYPEREVM_CHAIN_ID, type AppConfig, type ChainId } from './services/types';

/** UI state shared across screens: config, balances, global dialogs. */
class AppState {
  config: AppConfig | null = $state(null);
  balances: Record<ChainId, bigint | null> = $state({ [HYPEREVM_CHAIN_ID]: null, [ELYSIUM_CHAIN_ID]: null });
  walletDialogOpen = $state(false);
  howItWorksOpen = $state(false);

  async loadConfig() {
    try {
      this.config = await ferry.getConfig();
    } catch {
      // Screens fall back to INTERFACES.md defaults when config is unavailable.
    }
  }

  async refreshBalances() {
    if (!wallet.account) {
      this.balances = { [HYPEREVM_CHAIN_ID]: null, [ELYSIUM_CHAIN_ID]: null };
      return;
    }
    const [h, e] = await Promise.allSettled([wallet.getBalance(HYPEREVM_CHAIN_ID), wallet.getBalance(ELYSIUM_CHAIN_ID)]);
    this.balances = {
      [HYPEREVM_CHAIN_ID]: h.status === 'fulfilled' ? h.value : null,
      [ELYSIUM_CHAIN_ID]: e.status === 'fulfilled' ? e.value : null,
    };
  }
}

export const app = new AppState();
