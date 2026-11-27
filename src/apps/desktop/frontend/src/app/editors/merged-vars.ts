import { ChangeDetectionStrategy, Component, computed, inject, input, output } from '@angular/core';
import { TranslocoDirective, TranslocoService } from '@jsverse/transloco';
import { VarRow, WorkspaceService } from '@bindings/apps/desktop';
import { Preview, ResolvedVar } from '@bindings/internal/resolve';

import { VarMap, varMap } from '../core/preview.store';
import { Level, sourceLabel, usedInLabel } from '../core/source-label';
import { WorkspaceStore } from '../core/workspace.store';
import { VarInput } from '../shared/var-input';
import { FocusFollow } from './focus-follow';
import { MergedRow, mergeVars } from './merge';

/**
 * Variables at the current level (docs/specs, section 10): runtime ones
 * first, inherited ones in grey (a variable is not ignored, it is redefined:
 * typing in one defines it here), and in red those used but defined nowhere.
 */
@Component({
  selector: 'app-merged-vars',
  imports: [TranslocoDirective, VarInput],
  templateUrl: './merged-vars.html',
  styleUrl: './merged.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class MergedVars {
  readonly rows = input.required<VarRow[]>();
  readonly preview = input<Preview | null>(null);
  /** The level edited: in a folder, the folder's own rows are "here". */
  readonly level = input<Level>('request');
  readonly rowsChange = output<VarRow[]>();

  private readonly transloco = inject(TranslocoService);
  private readonly workspaces = inject(WorkspaceStore);
  private readonly focus = new FocusFollow();
  protected readonly vars = computed<VarMap>(() => varMap(this.preview()));
  protected readonly merged = computed(() =>
    mergeVars(this.rows(), this.preview()?.inheritedVars ?? [], this.preview()?.vars ?? []),
  );

  protected title(row: MergedRow<ResolvedVar>): string {
    const i = row.inherited;
    switch (row.kind) {
      case 'runtime':
        return this.transloco.translate('source.runtimeHint');
      case 'missing':
        return this.transloco.translate('source.undefined');
      case 'local':
        return i
          ? this.transloco.translate('source.replaces', {
              source: sourceLabel(this.transloco, i.source, this.level()),
            })
          : this.transloco.translate('source.here');
    }
    return sourceLabel(this.transloco, i?.source, this.level());
  }

  protected usedIn(v: ResolvedVar): string {
    return usedInLabel(this.transloco, v.usedIn);
  }

  protected local(row: MergedRow<ResolvedVar>): VarRow {
    return this.rows()[row.local!];
  }

  protected set(index: number, change: Partial<VarRow>, col?: string, event?: Event): void {
    if (col) {
      this.focus.follow(index, col, event);
    }
    this.rowsChange.emit(this.rows().map((r, i) => (i === index ? { ...r, ...change } : r)));
  }

  protected remove(index: number): void {
    this.rowsChange.emit(this.rows().filter((_, i) => i !== index));
  }

  protected add(row: Partial<VarRow> = {}, col?: string): void {
    if (col) {
      this.focus.follow(this.rows().length, col);
    }
    this.rowsChange.emit([...this.rows(), { key: '', value: '', enabled: true, ...row }]);
  }

  /** Typing in an inherited row defines the variable here. */
  protected override(i: ResolvedVar, value: string): void {
    this.add({ key: i.key, value }, 'value');
  }

  protected async deleteRuntime(v: ResolvedVar): Promise<void> {
    const ws = this.workspaces.currentId();
    await this.workspaces.run(() => WorkspaceService.DeleteRuntimeVar(ws, v.key));
  }

  protected masked(v: ResolvedVar): string {
    return v.secret ? '••••••' : v.value;
  }
}
