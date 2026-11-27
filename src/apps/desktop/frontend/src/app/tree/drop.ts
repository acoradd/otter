import { NodeKind, TreeNode } from '@bindings/apps/desktop';

import { NodeRef, TreeRow } from '../core/tree.store';

/** Where the pointer is over a row: above it, on it (folders), below it. */
export type DropPosition = 'before' | 'inside' | 'after';

/** A move the tree can ask Go for. */
export interface DropMove {
  parentId: number;
  /** Index among the nodes of the same kind in parentId, the dragged one left out; -1: last. */
  index: number;
}

/**
 * The drop position from the pointer's height in a row (ratio 0 at the top,
 * 1 at the bottom). A folder takes a drop inside in its middle half; a
 * request never takes one. Without a size (no layout), a folder takes it
 * inside and a request after.
 */
export function dropPosition(node: TreeNode, ratio: number): DropPosition {
  const folder = node.kind === NodeKind.NodeFolder;
  if (!Number.isFinite(ratio)) {
    return folder ? 'inside' : 'after';
  }
  if (folder) {
    return ratio < 0.25 ? 'before' : ratio > 0.75 ? 'after' : 'inside';
  }
  return ratio < 0.5 ? 'before' : 'after';
}

/**
 * The move a drop asks for, or null when the drop makes no sense (on itself,
 * a folder into its own subtree). Requests only live in folders; a request
 * dropped on a folder goes inside it, a folder dropped on a request goes into
 * that request's folder.
 */
export function dropMove(
  rows: readonly TreeRow[],
  children: ReadonlyMap<number, TreeNode[]>,
  dragged: NodeRef,
  targetIndex: number,
  position: DropPosition,
): DropMove | null {
  const target = rows[targetIndex];
  if (!target) {
    return null;
  }
  const t = target.node;
  if (t.kind === dragged.kind && t.id === dragged.id) {
    return null;
  }
  if (dragged.kind === NodeKind.NodeFolder && isInSubtree(rows, dragged.id, targetIndex)) {
    return null;
  }

  const targetIsFolder = t.kind === NodeKind.NodeFolder;
  if (dragged.kind === NodeKind.NodeRequest && targetIsFolder) {
    return { parentId: t.id, index: -1 };
  }
  if (dragged.kind === NodeKind.NodeFolder && !targetIsFolder) {
    return { parentId: target.parentId, index: -1 };
  }
  if (position === 'inside') {
    return { parentId: t.id, index: -1 };
  }
  // Same kind, before or after the target, among its siblings.
  const siblings = (children.get(target.parentId) ?? []).filter(
    (n) => n.kind === dragged.kind && n.id !== dragged.id,
  );
  const at = siblings.findIndex((n) => n.id === t.id);
  return { parentId: target.parentId, index: position === 'before' ? at : at + 1 };
}

/** Whether the row at index sits under the expanded folder folderId (or is it). */
function isInSubtree(rows: readonly TreeRow[], folderId: number, index: number): boolean {
  const start = rows.findIndex(
    (r) => r.node.kind === NodeKind.NodeFolder && r.node.id === folderId,
  );
  if (start < 0) {
    return false;
  }
  if (index === start) {
    return true;
  }
  const depth = rows[start].depth;
  for (let i = start + 1; i < rows.length && rows[i].depth > depth; i++) {
    if (i === index) {
      return true;
    }
  }
  return false;
}
