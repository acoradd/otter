# 1. Desktop stack: Wails v3, Go, Angular, SQLite

- Status: accepted
- Date: 2026-11-13

## Context

Otter is a desktop HTTP client for large collections (10 000 requests, six
levels deep). It must start fast, keep everything local in one file, show the
exact request before it is sent, and offer an English and French interface.

## Decision

- **Shell**: Wails v3, pinned to an exact version in `src/go.mod` and
  `frontend/package.json` (`@wailsio/runtime`). It gives native windows and
  Go ↔ TypeScript bindings generated from Go services, with a far smaller
  footprint than Electron.
- **Backend**: Go, one module in `src/`. Resolution, interpolation, request
  building and sending live in Go: the frontend never decides what is sent.
- **Frontend**: Angular (standalone components, signals, OnPush), in
  `src/apps/desktop/frontend`. Wails has no Angular template: the app started
  from the vanilla template, whose `frontend/` was replaced by an Angular CLI
  project; `npm run dev` / `build` are what the Wails tasks call.
- **Database**: SQLite through `modernc.org/sqlite` (pure Go, no CGO), with
  FTS5 for search.
- **i18n**: Transloco, not `@angular/localize`: the language switches at
  runtime without a reload, and Go's stable error codes map directly to
  dynamic keys (`errors.` + code). Transloco does not check keys at build
  time: CI runs `npm run i18n:check` and a spec that compiles every message.
- **Fonts and assets** are bundled (Inter, JetBrains Mono). Nothing is loaded
  from the network.

## Consequences

- Wails v3 upgrades are deliberate, one exact version at a time, with the Go
  module, the npm runtime and the release workflow's CLI bumped together.
- On Linux, Wails v3 builds against GTK 4 and WebKitGTK 6
  (`libgtk-4-dev`, `libwebkitgtk-6.0-dev`). Windows uses WebView2, macOS
  WKWebView. Drag and drop uses pointer events, the HTML5 API being
  unreliable across the three.
- The desktop Go package embeds `frontend/dist`: the frontend must be built
  before `go vet` or `go test` on that package.
