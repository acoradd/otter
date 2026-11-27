# 3. Tree: order, lazy loading and moves

- Status: accepted
- Date: 2026-11-27

## Context

Folders and requests live in two tables, each with its own `seq`. The tree
must stay fast with 10 000 requests six levels deep, and moving a node must
never leave the database inconsistent.

## Decision

- **Order**: in a folder, subfolders come first, then requests, each in
  their `seq` order (as in Postman and Bruno). Folders and requests are never
  interleaved, so each table keeps a simple order of its own.
- **Root**: the workspace root holds folders only. A request always has a
  folder (`request.folder_id NOT NULL`, as in the spec's schema).
- **Lazy loading**: `GetChildren(workspace, folder)` returns the direct
  children of one folder, with `hasChildren` so a folder can show its chevron
  without loading anything. The frontend caches loaded children and refetches
  the folders listed in `tree.changed`.
- **Moves**: `MoveNode(kind, id, parent, index)`, where `index` counts the
  nodes of the same kind in the target folder with the moved one left out
  (`-1`: last). Go renumbers the siblings in the same transaction, refuses a
  folder moved into itself or a descendant (`FOLDER_CYCLE`, recursive CTE)
  and anything crossing workspaces.
- **Names from the frontend**: the names of new nodes and copies ("New
  folder", "users (copy)") are translated by the frontend and passed to Go,
  which never produces display text.
- **Rendering**: the visible rows are flattened and rendered with
  `cdk-virtual-scroll-viewport` (fixed 26 px rows). Drag and drop uses
  pointer events; where a drop lands is a pure function (`tree/drop.ts`).

## Consequences

- Dropping a request on a folder always puts it inside, at the end; dropping
  a folder on a request puts it in that request's folder, after its folders.
- A tree with thousands of expanded rows only renders what is on screen.
