import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  computed,
  effect,
  inject,
  output,
  signal,
  untracked,
} from '@angular/core';
import { TranslocoDirective, TranslocoService } from '@jsverse/transloco';
import { Events } from '@wailsio/runtime';
import { EnvVarRow, EnvironmentView, WorkspaceService } from '@bindings/apps/desktop';

import { backendError } from '../core/backend-error';
import { errorKey } from '../core/error-key';
import { WorkspaceStore } from '../core/workspace.store';
import { NameForm } from '../shared/name-form';

/** A variable being edited, with a stable identity for the list. */
interface Row extends EnvVarRow {
  uid: number;
}

type Mode = 'idle' | 'create' | 'rename' | 'delete';

let nextUid = 1;
const toRows = (vars: EnvVarRow[] | null | undefined): Row[] =>
  (vars ?? []).map((v) => ({ ...v, uid: nextUid++ }));
const toVars = (rows: Row[]): EnvVarRow[] =>
  rows.map(({ key, value, secret, enabled }) => ({ key, value, secret, enabled }));

/**
 * The environments of the open workspace: the list, then the variables of
 * the selected one (key, value, secret, enabled), saved together.
 */
@Component({
  selector: 'app-environments',
  imports: [TranslocoDirective, NameForm],
  templateUrl: './environments.html',
  styleUrl: './environments.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class Environments {
  protected readonly store = inject(WorkspaceStore);
  private readonly transloco = inject(TranslocoService);
  readonly closed = output<void>();

  protected readonly environments = computed(() => this.store.view()?.environments ?? []);
  private readonly chosen = signal(0);
  /** The selected environment: the one chosen if it still exists, else the active one, else the first. */
  protected readonly selectedId = computed(() => {
    const list = this.environments();
    const active = this.store.view()?.activeEnvironmentId ?? 0;
    for (const id of [this.chosen(), active]) {
      if (list.some((e) => e.id === id)) {
        return id;
      }
    }
    return list[0]?.id ?? 0;
  });

  protected readonly env = signal<EnvironmentView | null>(null);
  protected readonly rows = signal<Row[]>([]);
  protected readonly dirty = computed(() => {
    const saved = this.env()?.vars ?? [];
    return JSON.stringify(toVars(this.rows())) !== JSON.stringify(saved);
  });
  protected readonly revealed = signal<ReadonlySet<number>>(new Set());
  protected readonly mode = signal<Mode>('idle');
  protected readonly saveError = signal<{ code: string; params?: Record<string, unknown> } | null>(
    null,
  );

  constructor() {
    effect(() => {
      const id = this.selectedId();
      untracked(() => void this.load(id));
    });
    const off = Events.On('environment.changed', (event) => {
      if (event.data === this.store.currentId() && !this.dirty()) {
        void this.load(this.selectedId());
      }
    });
    inject(DestroyRef).onDestroy(off);
  }

  private async load(id: number): Promise<void> {
    if (!id) {
      this.env.set(null);
      this.rows.set([]);
      return;
    }
    try {
      const view = await WorkspaceService.GetEnvironmentView(id);
      this.env.set(view);
      this.rows.set(toRows(view.vars));
      this.revealed.set(new Set());
      this.saveError.set(null);
    } catch (err) {
      this.store.error.set(backendError(err));
    }
  }

  protected select(id: number): void {
    this.chosen.set(id);
    this.mode.set('idle');
  }

  protected async create(name: string): Promise<void> {
    const workspaceId = this.store.currentId();
    let id = 0;
    if (
      await this.store.run(
        async () => (id = await WorkspaceService.CreateEnvironment(workspaceId, name)),
      )
    ) {
      this.chosen.set(id);
      this.mode.set('idle');
    }
  }

  protected async rename(name: string): Promise<void> {
    const id = this.selectedId();
    if (await this.store.run(() => WorkspaceService.RenameEnvironment(id, name))) {
      this.mode.set('idle');
    }
  }

  protected async delete(): Promise<void> {
    const id = this.selectedId();
    if (await this.store.run(() => WorkspaceService.DeleteEnvironment(id))) {
      this.mode.set('idle');
    }
  }

  protected addRow(): void {
    this.rows.update((rows) => [
      ...rows,
      { key: '', value: '', secret: false, enabled: true, uid: nextUid++ },
    ]);
  }

  protected removeRow(uid: number): void {
    this.rows.update((rows) => rows.filter((r) => r.uid !== uid));
  }

  protected edit(uid: number, change: Partial<EnvVarRow>): void {
    this.rows.update((rows) => rows.map((r) => (r.uid === uid ? { ...r, ...change } : r)));
  }

  protected toggleReveal(uid: number): void {
    this.revealed.update((set) => {
      const next = new Set(set);
      if (!next.delete(uid)) {
        next.add(uid);
      }
      return next;
    });
  }

  protected async save(): Promise<void> {
    const id = this.selectedId();
    try {
      await WorkspaceService.SaveEnvironmentVars(id, toVars(this.rows()));
      this.saveError.set(null);
      await this.load(id);
    } catch (err) {
      this.saveError.set(backendError(err));
    }
  }

  protected revert(): void {
    this.rows.set(toRows(this.env()?.vars));
    this.saveError.set(null);
  }

  protected errorKey(code: string): string {
    return errorKey(this.transloco, code);
  }
}
