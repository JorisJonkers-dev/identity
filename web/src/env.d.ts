/// <reference types="vite/client" />

// Types a .vue import for tools that read TypeScript alone (ESLint); vue-tsc types the real ones.
declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent
  export default component
}
