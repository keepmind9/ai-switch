/** Declare types for Vite environment variables (defaults to any if undeclared) */
interface ImportMetaEnv {
  readonly VITE_APP_TITLE: string
  readonly VITE_BASE_URL: string
  readonly VITE_ROUTER_HISTORY: "hash" | "html5"
  readonly VITE_PUBLIC_PATH: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
