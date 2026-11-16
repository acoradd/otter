# Contributing to Otter

Thanks for helping! Otter is a desktop HTTP client built for large
collections. Every change is judged by two questions: does it stay fast on
10 000 requests, and does the user see exactly what will be sent?

## Before you start

- **Read the functional scope**: [docs/specs/scope-v1.md](docs/specs/scope-v1.md).
- **Read the architecture rules** in [CLAUDE.md](CLAUDE.md) and the decisions
  in [docs/adr](docs/adr). They apply to humans and AI assistants alike.
- **Features**: open an issue first. v1 has a fixed scope; Git integration,
  collection runner, pre-request scripts, OAuth2 flows, GraphQL, WebSocket,
  gRPC and SSE are out of scope unless a maintainer says otherwise.
- **Security issues**: never in a public issue, see [SECURITY.md](SECURITY.md).

## Development setup

You need Go (version in [src/go.mod](src/go.mod)), Node 24, and the Wails CLI
at the version pinned in `src/go.mod`:

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0
```

On Linux, also: `libgtk-4-dev libwebkitgtk-6.0-dev libsoup-3.0-dev`.

```bash
cd src/apps/desktop/frontend && npm ci
cd src/apps/desktop && wails3 dev            # the app with hot reload
cd src/apps/desktop/frontend && npm run build
cd src && go test ./...                      # Go tests (build the frontend first)
cd src/apps/desktop/frontend && npm test     # frontend tests
cd src/apps/desktop/frontend && npm run i18n:check
```

After changing a Go service, regenerate and commit the bindings:

```bash
cd src/apps/desktop && wails3 generate bindings -clean=true -ts -i
```

## How the code is organized

- **Go is the source of truth.** Variable resolution, header layering,
  interpolation and request building live in Go. The Angular frontend renders
  views and sends commands.
- `src/internal/resolve` is pure (no I/O, no clock) and fully tested.
  `Send` builds the request from the same `Preview` the UI shows.
- Go returns **error codes**, never text. The frontend translates them.
- **Never block the user**: unresolved variables and cycles are warnings.

## Making a change

1. Fork the repository and create a branch from `main`.
2. Keep the change focused: one feature or fix per pull request.
3. Write tests: every change in `resolve`, `httpx` or `store` comes with Go
   tests (table-driven when it fits; HTTP against `httptest.Server`).
4. UI text goes through Transloco: add the key to **both**
   `src/i18n/en.json` and `src/i18n/fr.json` (French runs 15 to 20 % longer:
   check that layouts hold).
5. Significant architecture decisions get an ADR in `docs/adr`.
6. Run the checks above, then open a pull request against `main`.

### Commit messages

[Conventional Commits](https://www.conventionalcommits.org/):
`feat(resolve): …`, `fix(desktop): …`, `docs: …`, `refactor: …`. Write them in
English, in the imperative mood. Pull requests are squashed on merge: the
title becomes the commit message and the release notes, so write it for users.

### AI-assisted contributions

They are welcome, under the same rules: you are responsible for every line you
submit, and you have read and tested it.

## License

Otter is licensed under the [GNU GPL v3.0](LICENSE). By contributing, you
agree that your contributions are licensed under the same terms.
