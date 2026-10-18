/// <reference types="svelte" />
/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Keeper API base URL, e.g. http://localhost:8787. Unset = demo data. */
  readonly VITE_API_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
