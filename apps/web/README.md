# LapangGo Web

## Portfolio Demo Mode

The public portfolio build can run entirely in the browser without the Go API, PostgreSQL, Redis, Railway, Neon, or Xendit:

```bash
VITE_PORTFOLIO_DEMO=true npm run build
```

The demo uses deterministic synthetic data, one-click Customer/Owner/Superadmin sessions, and a namespaced `localStorage` state with a visible reset control. Its request adapter is fail-closed: unsupported paths return a local `501` response and are never forwarded to a backend or payment provider. A normal build keeps `VITE_PORTFOLIO_DEMO` unset or sets it to `false` and continues using the Go API.

For Vercel, create a separate project with root directory `apps/web`, build command `npm run build`, output directory `dist`, and only `VITE_PORTFOLIO_DEMO=true`. Do not configure database URLs, JWT secrets, Xendit keys, or callback tokens. The included `vercel.json` provides the SPA route fallback.

## Vite baseline

This template provides a minimal setup to get React working in Vite with HMR and some Oxlint rules.

Currently, two official plugins are available:

- [@vitejs/plugin-react](https://github.com/vitejs/vite-plugin-react/blob/main/packages/plugin-react) uses [Oxc](https://oxc.rs)
- [@vitejs/plugin-react-swc](https://github.com/vitejs/vite-plugin-react/blob/main/packages/plugin-react-swc) uses [SWC](https://swc.rs/)

## React Compiler

The React Compiler is not enabled on this template because of its impact on dev & build performances. To add it, see [this documentation](https://react.dev/learn/react-compiler/installation).

## Expanding the Oxlint configuration

If you are developing a production application, we recommend enabling type-aware lint rules by installing `oxlint-tsgolint` and editing `.oxlintrc.json`:

```json
{
  "$schema": "./node_modules/oxlint/configuration_schema.json",
  "plugins": ["react", "typescript", "oxc"],
  "options": {
    "typeAware": true
  },
  "rules": {
    "react/rules-of-hooks": "error",
    "react/only-export-components": ["warn", { "allowConstantExport": true }]
  }
}
```

See the [Oxlint rules documentation](https://oxc.rs/docs/guide/usage/linter/rules) for the full list of rules and categories.
