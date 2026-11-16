# Otter

A lightweight desktop HTTP client, an alternative to Postman and Bruno built
for large collections:

- **Lazy, virtualized tree**: thousands of requests, six levels deep, still
  instant to open and scroll.
- **Instant full-text search** across names, URLs and bodies.
- **What you see is what is sent**: variables and headers inherited from
  folders and the environment are resolved and shown before you hit Send.

Otter is 100 % local: one SQLite database, no account, no cloud sync.
The interface is in English and French.

> Status: early development. Nothing is released yet.

## Features (v1)

- Workspaces with nested folders and environments
- Variables and headers on folders and requests, with visible inheritance
- `{{var}}` interpolation in URL, headers and body, previewed live
- Bodies: JSON, XML, text, form URL-encoded, multipart with files, binary file
- Post-request scripts (Bruno-compatible `res` / `bru` API)
- Send history, cookies, proxy and TLS settings per workspace
- Encrypted secrets (key in the OS keyring)
- Import: OpenAPI (file or URL, with resync), Postman, Bruno, cURL, Otter
- Export: Otter, OpenAPI, Postman, Bruno
- Code generation: cURL, PowerShell, fetch, Python, Go, Java, C#, PHP, raw HTTP

The full functional scope is in [docs/specs/scope-v1.md](docs/specs/scope-v1.md)
(in French), the mockups in [docs/design/mockups.html](docs/design/mockups.html).

## Building from source

You need Go (version in [src/go.mod](src/go.mod)), Node 24 and the Wails CLI
at the version pinned in `src/go.mod`:

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0
```

On Linux, also `libgtk-4-dev libwebkitgtk-6.0-dev libsoup-3.0-dev`.

```bash
cd src/apps/desktop/frontend && npm ci
cd src/apps/desktop && wails3 dev      # run with hot reload
cd src/apps/desktop && wails3 build    # binary in src/apps/desktop/bin
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for tests and conventions.

## Repository layout

```
docs/specs/                 functional spec
docs/adr/                   architecture decision records
docs/design/                mockups
src/internal/               Go packages (resolve, httpx, store, codegen…)
src/apps/desktop/           Wails app and its Go services
src/apps/desktop/frontend/  Angular UI
```

## License

Otter is free software, licensed under the [GNU GPL v3.0](LICENSE).
Copyright © 2026 Thomas Da Rocha.
