# 4. Resolution and preview

- Status: accepted
- Date: 2026-11-27

## Context

The spec (sections 4 to 6) asks that what the user sees before sending is
exactly what is sent, that resolution lives in Go, and that Go never returns
display text.

## Decision

- **One pure function**: `resolve.Resolve(Input) Preview` takes the layers
  already loaded (runtime variables, request draft, folder chain, active
  environment, OS) and returns the preview. No I/O, no clock: the OS is an
  input, file content types come from a table of our own. Send will build the
  HTTP request from this preview only.
- **The draft, not the saved request**: `GetRequestPreview(id, draft)` and
  `GetFolderPreview(id, draft)` resolve what the editor holds, 150 ms after
  the last change, so the merged lists and highlights follow typing.
- **Sources are data**: `Source{kind, id, name, path, depth}`. The frontend
  writes the sentence ("Inherited from api › users"). Depth 0 is the
  request's folder (inherited) or, in the folder editor, the folder itself
  ("Defined here").
- **Warnings are codes with parameters** (`UNRESOLVED_VAR`, `VAR_CYCLE`,
  `EMPTY_HEADER`, `INVALID_URL`, `EMPTY_URL`), translated like errors.
- **The inherited state comes with the preview** (`inheritedVars`,
  `inheritedHeaders`: the current level's own rows and runtime variables left
  out). The merged lists are built from it in the frontend (`editors/merge.ts`,
  pure): an inherited row and the local row replacing it share one place and
  one identity, so typing in an inherited row turns it into a local one
  without losing the caret.
- **Rules worth stating**: within a layer, the last enabled row of a name
  wins; header names match without case and may hold `{{variables}}`; a
  variable used only by an ignored header or by a body that is not sent is
  not "missing"; an unresolved URL is not also reported as invalid; Host and
  Content-Length are computed and read-only unless a layer sets them;
  Content-Length counts bytes; multipart and file sizes are completed when
  sending (`computed`).
- **User-Agent**: Chrome stable's reduced User-Agent for the OS, with
  `resolve.ChromeMajorVersion` the only number to update.

## Consequences

- `{{var}}` highlighting in one-line fields draws the text in a mirror under a
  transparent input; it relies on the monospace font to find the hovered
  character.
- Resolution is covered by table-driven tests in `internal/resolve` (98 %).
