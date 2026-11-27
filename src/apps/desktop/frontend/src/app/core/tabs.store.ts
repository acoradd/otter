import { DestroyRef, Injectable, computed, inject, signal } from '@angular/core';
import { Events } from '@wailsio/runtime';
import { EditorService, FolderDraft, NodeKind, RequestDraft } from '@bindings/apps/desktop';

import { BackendError, backendError } from './backend-error';
import { WorkspaceStore } from './workspace.store';

export type TabKind = 'request' | 'folder';

/** An open editor: what Go saved, and what the user is editing. */
export interface Tab<D = RequestDraft | FolderDraft> {
  kind: TabKind;
  id: number;
  workspaceId: number;
  name: string;
  /** Folders from the root down to the request's folder, or to the folder itself. */
  path: string[];
  saved: D;
  draft: D;
  error: BackendError | null;
}

export type RequestTab = Tab<RequestDraft>;
export type FolderTab = Tab<FolderDraft>;

export const tabKey = (t: { kind: TabKind; id: number }): string => `${t.kind}:${t.id}`;
export const isDirty = (t: Tab): boolean => JSON.stringify(t.draft) !== JSON.stringify(t.saved);

/**
 * The editors open in tabs, per workspace. Drafts live here until saved
 * (Ctrl+S); Go's events refresh what is not being edited.
 */
@Injectable({ providedIn: 'root' })
export class TabsStore {
  private readonly workspaces = inject(WorkspaceStore);

  private readonly all = signal<Tab[]>([]);
  /** The active tab of each workspace. */
  private readonly activeKeys = signal<ReadonlyMap<number, string>>(new Map());

  readonly tabs = computed(() =>
    this.all().filter((t) => t.workspaceId === this.workspaces.currentId()),
  );
  readonly activeKey = computed(() => this.activeKeys().get(this.workspaces.currentId()) ?? '');
  readonly active = computed(() => this.tabs().find((t) => tabKey(t) === this.activeKey()) ?? null);

  constructor() {
    const offs = [
      Events.On('request.changed', (e) => void this.refresh('request', e.data)),
      Events.On('folder.changed', (e) => void this.refresh('folder', e.data)),
      // Renamed, moved, deleted: names, paths and existence of open tabs.
      Events.On('tree.changed', (e) => {
        for (const t of this.all().filter((t) => t.workspaceId === e.data.workspaceId)) {
          void this.refresh(t.kind, t.id);
        }
      }),
    ];
    inject(DestroyRef).onDestroy(() => offs.forEach((off) => off()));
  }

  /** Opens an editor, or brings it to the front. */
  async open(kind: TabKind, id: number): Promise<void> {
    const key = tabKey({ kind, id });
    if (!this.all().some((t) => tabKey(t) === key)) {
      const tab = await this.load(kind, id);
      if (!tab) {
        return;
      }
      if (!this.all().some((t) => tabKey(t) === key)) {
        this.all.update((tabs) => [...tabs, tab]);
      }
    }
    this.activate(key);
  }

  activate(key: string): void {
    this.activeKeys.update((m) => new Map(m).set(this.workspaces.currentId(), key));
  }

  /** Replaces the draft of a tab (the editors call it on every change). */
  edit<D extends RequestDraft | FolderDraft>(key: string, change: (draft: D) => D): void {
    this.all.update((tabs) =>
      tabs.map((t) => (tabKey(t) === key ? { ...t, draft: change(t.draft as D), error: null } : t)),
    );
  }

  revert(key: string): void {
    this.all.update((tabs) =>
      tabs.map((t) => (tabKey(t) === key ? { ...t, draft: t.saved, error: null } : t)),
    );
  }

  /** Saves a tab. Go checks it; a refusal stays on the tab as an error code. */
  async save(key: string): Promise<boolean> {
    const tab = this.all().find((t) => tabKey(t) === key);
    if (!tab || !isDirty(tab)) {
      return true;
    }
    try {
      if (tab.kind === 'request') {
        await EditorService.SaveRequest(tab.id, tab.draft as RequestDraft);
      } else {
        await EditorService.SaveFolder(tab.id, tab.draft as FolderDraft);
      }
    } catch (err) {
      this.setTab(key, (t) => ({ ...t, error: backendError(err) }));
      return false;
    }
    // What Go stored (normalised: method upper-cased, blank rows dropped).
    const fresh = await this.load(tab.kind, tab.id);
    if (fresh) {
      this.setTab(key, () => fresh);
    }
    return true;
  }

  /** Closes a tab, unsaved changes included: ask before calling it. */
  close(key: string): void {
    const tabs = this.tabs();
    const index = tabs.findIndex((t) => tabKey(t) === key);
    this.all.update((all) => all.filter((t) => tabKey(t) !== key));
    if (this.activeKey() === key) {
      const next = tabs[index + 1] ?? tabs[index - 1];
      this.activate(next ? tabKey(next) : '');
    }
  }

  /** Refetches a tab from Go: all of it when clean, its name and path when edited. Gone: closed. */
  private async refresh(kind: TabKind, id: number): Promise<void> {
    const key = tabKey({ kind, id });
    if (!this.all().some((t) => tabKey(t) === key)) {
      return;
    }
    const fresh = await this.load(kind, id, true);
    if (fresh === 'gone') {
      this.close(key);
      return;
    }
    if (fresh) {
      this.setTab(key, (t) =>
        isDirty(t)
          ? { ...t, name: fresh.name, path: fresh.path, saved: fresh.saved }
          : { ...fresh, error: t.error },
      );
    }
  }

  private async load(kind: TabKind, id: number): Promise<Tab | null>;
  private async load(kind: TabKind, id: number, quiet: true): Promise<Tab | null | 'gone'>;
  private async load(kind: TabKind, id: number, quiet = false): Promise<Tab | null | 'gone'> {
    try {
      const view =
        kind === 'request'
          ? await EditorService.GetRequestView(id)
          : await EditorService.GetFolderView(id);
      return {
        kind,
        id,
        workspaceId: view.workspaceId,
        name: view.name,
        path: view.path ?? [],
        saved: view.draft,
        draft: view.draft,
        error: null,
      };
    } catch (err) {
      const e = backendError(err);
      if (quiet && e.code === 'NOT_FOUND') {
        return 'gone';
      }
      this.workspaces.error.set(e);
      return null;
    }
  }

  private setTab(key: string, change: (t: Tab) => Tab): void {
    this.all.update((tabs) => tabs.map((t) => (tabKey(t) === key ? change(t) : t)));
  }
}

/** The tab kind of a tree node. */
export const kindOf = (kind: NodeKind): TabKind =>
  kind === NodeKind.NodeFolder ? 'folder' : 'request';
