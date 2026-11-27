import { DestroyRef, Injectable, effect, inject, signal, untracked } from '@angular/core';
import { Events } from '@wailsio/runtime';
import { FolderDraft, PreviewService, RequestDraft } from '@bindings/apps/desktop';
import { Preview, ResolvedVar } from '@bindings/internal/resolve';

import { Tab, TabsStore, tabKey } from './tabs.store';
import { WorkspaceStore } from './workspace.store';

/** Delay between the last change of a draft and its preview, in ms. */
export const PREVIEW_DEBOUNCE = 150;

/** The variables of a preview by name, for highlighting and hovering {{name}}. */
export type VarMap = ReadonlyMap<string, ResolvedVar>;

export function varMap(preview: Preview | null): VarMap {
  return new Map((preview?.vars ?? []).map((v) => [v.key, v]));
}

/**
 * The preview of the active editor, as Go resolves it from the draft:
 * recomputed shortly after each change, and when what it inherits changes.
 */
@Injectable({ providedIn: 'root' })
export class PreviewStore {
  private readonly tabs = inject(TabsStore);
  private readonly workspaces = inject(WorkspaceStore);

  /** The preview of each tab, by tab key. */
  readonly previews = signal<ReadonlyMap<string, Preview>>(new Map());
  private timer: ReturnType<typeof setTimeout> | undefined;
  /** Answers can come back out of order: only the latest counts. */
  private sequence = 0;

  constructor() {
    effect(() => {
      const tab = this.tabs.active();
      if (tab) {
        // Tracks the draft: any edit schedules a preview.
        tab.draft;
        untracked(() => this.schedule(tab));
      }
    });
    const refresh = (workspaceId: number) => {
      const tab = this.tabs.active();
      if (tab && tab.workspaceId === workspaceId) {
        this.schedule(tab, 0);
      }
    };
    const current = () => this.workspaces.currentId();
    const offs = [
      Events.On('environment.changed', (e) => refresh(e.data)),
      Events.On('runtimeVars.changed', (e) => refresh(e.data)),
      Events.On('folder.changed', () => refresh(current())),
      Events.On('tree.changed', (e) => refresh(e.data.workspaceId)),
    ];
    const destroyRef = inject(DestroyRef);
    destroyRef.onDestroy(() => {
      offs.forEach((off) => off());
      clearTimeout(this.timer);
    });
  }

  /** The preview of a tab, null until the first one arrives. */
  of(tab: Tab | null): Preview | null {
    return tab ? (this.previews().get(tabKey(tab)) ?? null) : null;
  }

  private schedule(tab: Tab, delay = PREVIEW_DEBOUNCE): void {
    clearTimeout(this.timer);
    const first = !this.previews().has(tabKey(tab));
    this.timer = setTimeout(() => void this.load(tab), first ? 0 : delay);
  }

  private async load(tab: Tab): Promise<void> {
    const seq = ++this.sequence;
    try {
      const preview =
        tab.kind === 'request'
          ? await PreviewService.GetRequestPreview(tab.id, tab.draft as RequestDraft)
          : await PreviewService.GetFolderPreview(tab.id, tab.draft as FolderDraft);
      if (seq === this.sequence) {
        this.previews.update((m) => new Map(m).set(tabKey(tab), preview));
      }
    } catch {
      // The tab is gone, or Go refused: the editor shows the last preview.
    }
  }
}
