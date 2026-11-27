import { DestroyRef, Injectable, computed, effect, inject, signal, untracked } from '@angular/core';
import { Events } from '@wailsio/runtime';
import { WorkspaceItem, WorkspaceService, WorkspaceView } from '@bindings/apps/desktop';

import { BackendError, backendError } from './backend-error';
import { AppSettings } from './settings.store';

/**
 * The workspaces and the open one, as Go sees them. Commands wait for Go
 * (no optimistic update); Go's events trigger the refetch.
 */
@Injectable({ providedIn: 'root' })
export class WorkspaceStore {
  private readonly settings = inject(AppSettings);

  readonly workspaces = signal<WorkspaceItem[]>([]);
  readonly view = signal<WorkspaceView | null>(null);
  readonly loaded = signal(false);
  readonly error = signal<BackendError | null>(null);

  /** The open workspace: the last one opened if it still exists, else the first. */
  readonly currentId = computed(() => {
    const list = this.workspaces();
    const last = this.settings.value()?.lastWorkspaceId ?? 0;
    return list.some((w) => w.id === last) ? last : (list[0]?.id ?? 0);
  });

  constructor() {
    // The view follows the open workspace, whatever changed it (the list,
    // the settings).
    effect(() => {
      const id = this.currentId();
      untracked(() => void this.refreshView(id));
    });
    const offs = [
      Events.On('workspaces.changed', () => void this.refresh()),
      Events.On('environment.changed', (event) => {
        if (event.data === this.currentId()) {
          void this.refreshView(event.data);
        }
      }),
    ];
    inject(DestroyRef).onDestroy(() => offs.forEach((off) => off()));
  }

  /** Loads the list of workspaces; the open one's view follows. */
  async refresh(): Promise<void> {
    try {
      this.workspaces.set((await WorkspaceService.ListWorkspaces()) ?? []);
      this.loaded.set(true);
    } catch (err) {
      this.error.set(backendError(err));
    }
  }

  private async refreshView(id: number): Promise<void> {
    if (!id) {
      this.view.set(null);
      return;
    }
    try {
      const view = await WorkspaceService.GetWorkspaceView(id);
      if (id === this.currentId()) {
        this.view.set(view);
      }
    } catch (err) {
      this.error.set(backendError(err));
    }
  }

  create(name: string): Promise<boolean> {
    return this.run(() => WorkspaceService.CreateWorkspace(name));
  }

  open(id: number): Promise<boolean> {
    return this.run(() => WorkspaceService.OpenWorkspace(id));
  }

  rename(id: number, name: string): Promise<boolean> {
    return this.run(() => WorkspaceService.RenameWorkspace(id, name));
  }

  delete(id: number): Promise<boolean> {
    return this.run(() => WorkspaceService.DeleteWorkspace(id));
  }

  setActiveEnvironment(envId: number): Promise<boolean> {
    return this.run(() => WorkspaceService.SetActiveEnvironment(this.currentId(), envId));
  }

  /** Runs a command; on failure keeps its error code for the alert. */
  async run(call: () => Promise<unknown>): Promise<boolean> {
    try {
      await call();
      this.error.set(null);
      return true;
    } catch (err) {
      this.error.set(backendError(err));
      return false;
    }
  }
}
