/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly DEV: boolean;
  readonly PROD: boolean;
  readonly MODE: string;
  /** Show validation nav item (default: hidden). Set to 'true' to enable. */
  readonly VITE_SHOW_VALIDATION?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
