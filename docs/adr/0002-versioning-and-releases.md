# 2. Versioning and releases

- Status: accepted
- Date: 2026-11-13

## Decision

The desktop app is versioned `MAJOR.MINOR.PATCH`:

| Part | Changes when |
| --- | --- |
| MAJOR | The database or the native export format stops being readable by older versions without a migration path. |
| MINOR | Features or functional fixes. |
| PATCH | Security fixes and dependency updates only (Renovate PRs). |

- A release is a tag `desktop-vX.Y.Z` on a commit of `main`. The release
  workflow checks it, stamps the version (`version.txt` and the packaging
  metadata) and publishes the GitHub release.
- `version.txt` stays `dev` in `main`: builds outside a release report `dev`.
- Release notes are generated from Conventional Commits by git-cliff.
- Dependencies are kept up to date by Renovate, grouped by ecosystem (Wails
  Go module, npm runtime and CLI move together), held 7 days after release
  except for vulnerability fixes.

## Consequences

- Tags are the source of truth for versions; nothing in `main` is edited by
  hand to release.
- A merged Renovate PR does not release anything by itself: the next tag
  bumps PATCH.
