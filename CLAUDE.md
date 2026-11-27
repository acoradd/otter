# CLAUDE.md

**Otter**: lightweight desktop HTTP client, an alternative to Postman and Bruno built for large collections (lazy tree, instant full-text search, variable and header resolution shown before sending).
The functional spec lives in `docs/specs/scope-v1.md`. Read it before any feature work.

## Scope

- **v1**: desktop app, 100 % local. One SQLite database, no account, no cloud sync. UI in English and French.
- Out of scope for v1 (do not build unless asked): Git integration, collection runner, pre-request scripts, dedicated auth flows (OAuth2, Digest, NTLM), GraphQL, WebSocket, gRPC, SSE.

## Stack

- **Desktop shell**: Wails v3 (pin the exact version in `go.mod`)
- **Backend**: Go, single module in `src/`
- **Frontend**: Angular, in `src/apps/desktop/frontend`
- **i18n**: Transloco, runtime language switch, `en` and `fr`
- **Database**: SQLite via `modernc.org/sqlite` (pure Go, no CGO), FTS5
- **Scripts**: `github.com/dop251/goja`
- **Editor**: `monaco-editor`, wrapped in our own Angular component

## Repository layout

All source code lives under `src/`. The repo root only holds docs, CI and project files.

```
CLAUDE.md, README.md, LICENSE
docs/specs/                   functional spec
docs/adr/                     architecture decision records
.github/workflows/            CI and release
src/go.mod                    single Go module
src/internal/apperr/          stable error codes (Code, *Error)
src/internal/resolve/         pure: interpolation, variable and header resolution → Preview
src/internal/httpx/           send from a Preview: client, redirects, URL encoding, bodies (multipart and files streamed), decompression
src/internal/codegen/         code generation from a Preview, one generator per language (golden tests)
src/internal/script/          goja runtime, `res` and `bru` API
src/internal/store/           SQLite access, append-only migrations, queries, FTS
src/internal/secrets/         OS keyring + AES-GCM
src/internal/importexport/    native, OpenAPI (file, URL, resync, export), Bruno, Postman, cURL parser
src/internal/version/         app version
src/apps/desktop/             Wails app: main.go, services (bound to the frontend), window
src/apps/desktop/frontend/    Angular app
```

## Architecture rules

- **Go is the source of truth.** Variable resolution, header layering, interpolation and request building live in Go. The frontend never resolves a variable or decides which header is sent.
- **`src/internal/resolve` is pure**: no I/O, no SQLite, no Wails imports, no clock reads. It takes loaded layers and returns a `Preview`. Fully unit-tested, table-driven.
- **What you see is what is sent.** `Send` builds the HTTP request only from the `Preview` returned by the same code path as `GetPreview`.
- **Commands and queries are separate.**
    - Commands express intent (`CreateFolder`, `MoveNode`, `SaveRequest`, `SetFolderHeaders`, `SetActiveEnvironment`, `SendRequest`, `CancelRequest`) and return OK or an error code.
    - Queries return ready-to-render views (`GetChildren`, `GetRequestView`, `GetFolderView`, `GetPreview`, `Search`, `GetHistory`).
    - After a change, Go emits a Wails event (`tree.changed` with the folder ID, `request.changed`, `environment.changed`, `runtimeVars.changed`). The frontend refetches what it shows.
- **No optimistic updates** in the frontend. Calls are in-process and fast, wait for the result.
- **Go never returns human-readable text.** Errors are stable codes (`FOLDER_CYCLE`, `INVALID_URL`, `KEYRING_UNAVAILABLE`, `SCRIPT_TIMEOUT`) as `apperr.Code` wrapped in `*apperr.Error`. The app-level `MarshalError` sends them as the call's `cause` (`{code, ...}`); anything else becomes `INTERNAL` and is logged. The frontend reads it with `backendError()` and shows `errors.<CODE>`. Preview warnings are codes with parameters too.
- **Services take their dependencies** (store, `now`, `emit`, HTTP transport, dialogs) so they are tested without Wails.
- **Lazy tree**: expanding a folder loads its direct children only. The frontend caches loaded children and drops them on `tree.changed`.
- **Never block the user.** Unresolved variables, empty headers after interpolation and cycles are warnings in the preview, the request can still be sent.

## Data

- One database `otter.db` in `os.UserConfigDir()/Otter`. WAL mode, `foreign_keys=ON`.
- Recognized by its SQLite `application_id` (`OTTR`), versioned by `user_version`. Migrations are append-only lists in `internal/store`, applied on open.
- Open SQLite with a plain path, never a `file:` URI (Windows paths, `#` and `%` in names).
- Secrets (`env_var.secret = 1`) are encrypted with AES-GCM. The key lives in the OS keyring, never next to the database.
- Settings (theme, language, last workspace, panel sizes) live in a `setting` table and are broadcast with `settings.changed`.

## Conventions

- All code, comments, commit messages, issues and docs in English. The functional spec may stay in French.
- i18n keys in English, namespaced by screen: `request.send`, `preview.ignoredBy`. ICU plurals. No hardcoded UI strings.
- French labels run 15 to 20 % longer than English: layouts must not break.
- Commits: Conventional Commits (`feat(resolve): ...`, `fix(desktop): ...`).
- Every change in `resolve`, `httpx` or `store` comes with tests. Prefer table-driven tests. HTTP behaviour is tested against `httptest.Server`.
- Record significant architecture decisions as ADRs in `docs/adr/`.
- Frontend: standalone components, signals, `ChangeDetectionStrategy.OnPush`, no NgRx.
- Drag and drop uses pointer events, not HTML5 drag and drop (unreliable across WebKitGTK, WebView2 and WKWebView).

## Commands

Prerequisites: Go (version in `src/go.mod`), Node 24, the Wails CLI pinned to the same version as `src/go.mod`
(`go install github.com/wailsapp/wails/v3/cmd/wails3@<version>`).
On Linux: `libgtk-4-dev libwebkitgtk-6.0-dev libsoup-3.0-dev`.

```
cd src/apps/desktop && wails3 dev             # run the desktop app with hot reload
cd src/apps/desktop && wails3 build           # production binary in src/apps/desktop/bin
cd src/apps/desktop && wails3 generate bindings -clean=true -ts -i   # after changing a Go service
cd src && go test ./...                       # all Go tests (build the frontend first: the desktop package embeds frontend/dist)
cd src/apps/desktop/frontend && npm test      # frontend unit tests (Vitest)
cd src/apps/desktop/frontend && npm run i18n:check   # en.json and fr.json must share the same keys
cd src/apps/desktop && OTTER_FIXTURES=$PWD/frontend/src/testing/fixtures go test -run TestWriteFrontendFixtures .   # after changing a view
```

Frontend tests render each screen with Go faked: `src/testing/backend.ts` (`FakeBackend`: a Wails transport answering each binding by name and recording the calls) and `src/testing/render.ts` (real translations, `click`, `drag`…). Views come from `src/testing/fixtures/*.json`, written by the Go test above from a demo workspace (fixed clock, local `httptest` server): regenerate them when a view changes. Every screen has a spec.

Generated bindings (`frontend/bindings`) are committed. Import them through the `@bindings/*` path alias.

A dev command seeds a large demo workspace (10 000 requests, 6 levels deep) to check the tree and search stay fast: `cd src && go run ./apps/seed` (close Otter first; `-db`, `-requests`, `-depth`, `-branching`). `TestLargeTreeStaysFast` in `internal/store` checks the same size in CI.

## Versions and releases

- Versioned `MAJOR.MINOR.PATCH`. A release is a tag `desktop-vX.Y.Z` on a commit of `main`; the release workflow stamps the version and publishes the GitHub release. `version.txt` stays `dev` in `main`.
- Renovate keeps dependencies up to date. Keep the Wails Go module, `@wailsio/runtime` and the release workflow's Wails version on the same version.

## Design

Colors are CSS custom properties in `frontend/src/styles.scss`. Shared building blocks live there (`.card`, `.btn`, `.seg`, `.pill-*`, `.empty`, `.alert`…): use them before adding component styles. Repeated patterns are mixins in `src/styles/_mixins.scss`.
The main window has no native title bar on Windows and Linux (`Frameless`): the app's top bar drags the window (`--wails-draggable: drag`, controls `no-drag`) and draws minimise / maximise / close. On macOS the native traffic lights sit over its left end (`MacTitleBarHiddenInset`).
Light and dark themes, following the system by default.
