import type { FerryService, WalletService } from './types';
import { createLiveServices } from './live';
import { createMockServices } from './mock';

const apiUrl: string | undefined = import.meta.env.VITE_API_URL;

const services = apiUrl ? createLiveServices(apiUrl) : createMockServices();

export const ferry: FerryService = services.ferry;
export const wallet: WalletService = services.wallet;
export const isDemo = ferry.mode === 'mock';
