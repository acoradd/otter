import { DestroyRef, Injectable, computed, effect, inject, signal, untracked } from '@angular/core';
import { Events } from '@wailsio/runtime';
import { NodeKind, TreeNode, TreeService } from '@bindings/apps/desktop';

import { WorkspaceStore } from './workspace.store';

/** A node of the tree: a folder or a request. */
export interface NodeRef {
  kind: NodeKind;
  id: number;
}

/** A visible row of the tree, flattened for the virtual scroll. */
export interface TreeRow {
  node: TreeNode;
  /** 0 at the root. */
  depth: number;
  /** The folder holding the node; 0 for the root. */
  parentId: number;
  expanded: boolean;
  loading: boolean;
}

export const ROOT = 0;

export const nodeKey = (n: NodeRef): string => `${n.kind}:${n.id}`;
export const sameNode = (a: NodeRef | null, b: NodeRef | null): boolean =>
  !!a && !!b && a.kind === b.kind && a.id === b.id;

/**
 * The tree of the open workspace, loaded lazily: expanding a folder loads its
 * direct children only. Loaded children are cached and refetched when Go
 * says their folder changed (tree.changed).
 */
@Injectable({ providedIn: 'root' })
export class TreeStore {
  private readonly workspaces = inject(WorkspaceStore);

  /** Loaded children by folder ID (ROOT for the root). */
  readonly children = signal<ReadonlyMap<number, TreeNode[]>>(new Map());
  readonly expanded = signal<ReadonlySet<number>>(new Set());
  readonly loading = signal<ReadonlySet<number>>(new Set());
  readonly selected = signal<NodeRef | null>(null);
  /** The node whose name is being edited in place. */
  readonly renaming = signal<NodeRef | null>(null);

  readonly workspaceId = this.workspaces.currentId;

  /** The visible rows: the root's children, and those of every expanded folder. */
  readonly rows = computed(() => {
    const children = this.children();
    const expanded = this.expanded();
    const loading = this.loading();
    const rows: TreeRow[] = [];
    const walk = (folderId: number, depth: number) => {
      for (const node of children.get(folderId) ?? []) {
        const open = node.kind === NodeKind.NodeFolder && expanded.has(node.id);
        rows.push({
          node,
          depth,
          parentId: folderId,
          expanded: open,
          loading: loading.has(node.id),
        });
        if (open) {
          walk(node.id, depth + 1);
        }
      }
    };
    walk(ROOT, 0);
    return rows;
  });

  readonly rootLoaded = computed(() => this.children().has(ROOT));

  constructor() {
    effect(() => {
      const ws = this.workspaceId();
      untracked(() => this.reset(ws));
    });
    const off = Events.On('tree.changed', (event) => {
      if (event.data.workspaceId !== this.workspaceId()) {
        return;
      }
      for (const id of event.data.folderIds ?? []) {
        if (this.children().has(id)) {
          void this.load(id);
        }
      }
    });
    inject(DestroyRef).onDestroy(off);
  }

  private reset(workspaceId: number): void {
    this.children.set(new Map());
    this.expanded.set(new Set());
    this.selected.set(null);
    this.renaming.set(null);
    if (workspaceId) {
      void this.load(ROOT);
    }
  }

  /** Loads (or reloads) the children of a folder. */
  async load(folderId: number): Promise<void> {
    const ws = this.workspaceId();
    this.loading.update((s) => new Set(s).add(folderId));
    try {
      const nodes = (await TreeService.GetChildren(ws, folderId)) ?? [];
      if (ws !== this.workspaceId()) {
        return;
      }
      const before = this.children().get(folderId) ?? [];
      this.children.update((m) => new Map(m).set(folderId, nodes));
      // A folder gone from here is no longer expanded.
      const gone = before.filter(
        (n) =>
          n.kind === NodeKind.NodeFolder && !nodes.some((x) => x.id === n.id && x.kind === n.kind),
      );
      if (gone.length) {
        this.expanded.update((s) => {
          const next = new Set(s);
          gone.forEach((n) => next.delete(n.id));
          return next;
        });
      }
    } catch (err) {
      void this.workspaces.run(() => Promise.reject(err));
    } finally {
      this.loading.update((s) => {
        const next = new Set(s);
        next.delete(folderId);
        return next;
      });
    }
  }

  /** Expands or collapses a folder; expanding loads its children once. */
  toggle(folderId: number): void {
    this.setExpanded(folderId, !this.expanded().has(folderId));
  }

  setExpanded(folderId: number, open: boolean): void {
    if (open === this.expanded().has(folderId)) {
      return;
    }
    this.expanded.update((s) => {
      const next = new Set(s);
      if (open) {
        next.add(folderId);
      } else {
        next.delete(folderId);
      }
      return next;
    });
    if (open && !this.children().has(folderId)) {
      void this.load(folderId);
    }
  }

  async createFolder(parentId: number, name: string): Promise<void> {
    let id = 0;
    const ws = this.workspaceId();
    if (
      await this.workspaces.run(
        async () => (id = await TreeService.CreateFolder(ws, parentId, name)),
      )
    ) {
      this.afterCreate(parentId, { kind: NodeKind.NodeFolder, id });
    }
  }

  async createRequest(folderId: number, name: string): Promise<void> {
    let id = 0;
    if (
      await this.workspaces.run(async () => (id = await TreeService.CreateRequest(folderId, name)))
    ) {
      this.afterCreate(folderId, { kind: NodeKind.NodeRequest, id });
    }
  }

  /** Shows a new node and lets the user name it. */
  private afterCreate(parentId: number, node: NodeRef): void {
    if (parentId !== ROOT) {
      this.setExpanded(parentId, true);
    }
    this.selected.set(node);
    this.renaming.set(node);
  }

  async rename(node: NodeRef, name: string): Promise<boolean> {
    const ok = await this.workspaces.run(() => TreeService.RenameNode(node.kind, node.id, name));
    if (ok) {
      this.renaming.set(null);
    }
    return ok;
  }

  async duplicate(node: NodeRef, name: string): Promise<void> {
    let id = 0;
    if (
      await this.workspaces.run(
        async () => (id = await TreeService.DuplicateNode(node.kind, node.id, name)),
      )
    ) {
      this.selected.set({ kind: node.kind, id });
    }
  }

  async delete(node: NodeRef): Promise<void> {
    if (await this.workspaces.run(() => TreeService.DeleteNode(node.kind, node.id))) {
      if (sameNode(this.selected(), node)) {
        this.selected.set(null);
      }
    }
  }

  /** Moves a node into parentId at index among the nodes of its kind there (-1: last). */
  async move(node: NodeRef, parentId: number, index: number): Promise<boolean> {
    const ok = await this.workspaces.run(() =>
      TreeService.MoveNode(node.kind, node.id, parentId, index),
    );
    if (ok && parentId !== ROOT) {
      this.setExpanded(parentId, true);
    }
    return ok;
  }
}
