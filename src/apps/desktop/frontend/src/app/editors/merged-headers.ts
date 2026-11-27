import { ChangeDetectionStrategy, Component, computed, inject, input, output } from '@angular/core';
import { TranslocoDirective, TranslocoService } from '@jsverse/transloco';
import { HeaderRow } from '@bindings/apps/desktop';
import { Preview, ResolvedHeader } from '@bindings/internal/resolve';

import { VarMap, varMap } from '../core/preview.store';
import { Level, sourceLabel } from '../core/source-label';
import { VarInput } from '../shared/var-input';
import { FocusFollow } from './focus-follow';
import { MergedRow, mergeHeaders } from './merge';

/**
 * Headers at the current level, inherited ones included (docs/specs, section
 * 10): grey rows come from a parent folder or are automatic; changing one
 * creates a row here. Unchecking an inherited header ignores it here.
 */
@Component({
  selector: 'app-merged-headers',
  imports: [TranslocoDirective, VarInput],
  templateUrl: './merged-headers.html',
  styleUrl: './merged.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class MergedHeaders {
  readonly rows = input.required<HeaderRow[]>();
  readonly preview = input<Preview | null>(null);
  /** The level edited: in a folder, the folder's own rows are "here". */
  readonly level = input<Level>('request');
  readonly rowsChange = output<HeaderRow[]>();

  private readonly transloco = inject(TranslocoService);
  private readonly focus = new FocusFollow();
  protected readonly vars = computed<VarMap>(() => varMap(this.preview()));
  protected readonly merged = computed(() =>
    mergeHeaders(
      this.rows(),
      this.preview()?.inheritedHeaders ?? [],
      this.preview()?.headers ?? [],
    ),
  );

  protected title(row: MergedRow<ResolvedHeader>): string {
    const i = row.inherited;
    if (row.kind === 'readonly') {
      return this.transloco.translate('source.computed');
    }
    if (row.kind === 'local') {
      return i
        ? this.transloco.translate('source.replaces', {
            source: sourceLabel(this.transloco, i.source, this.level()),
          })
        : this.transloco.translate('source.here');
    }
    const from = sourceLabel(this.transloco, i!.source, this.level());
    return i!.ignored
      ? `${from} · ${this.transloco.translate('source.ignoredBy', { source: sourceLabel(this.transloco, i!.ignoredBy, this.level()) })}`
      : from;
  }

  protected local(row: MergedRow<ResolvedHeader>): HeaderRow {
    return this.rows()[row.local!];
  }

  protected set(index: number, change: Partial<HeaderRow>, col?: string, event?: Event): void {
    if (col) {
      this.focus.follow(index, col, event);
    }
    this.rowsChange.emit(this.rows().map((r, i) => (i === index ? { ...r, ...change } : r)));
  }

  protected remove(index: number): void {
    this.rowsChange.emit(this.rows().filter((_, i) => i !== index));
  }

  protected add(row: Partial<HeaderRow> = {}, col?: string, event?: Event): void {
    if (col) {
      this.focus.follow(this.rows().length, col, event);
    }
    this.rowsChange.emit([
      ...this.rows(),
      { key: '', value: '', ignore: false, enabled: true, ...row },
    ]);
  }

  /** Typing in an inherited row replaces it here. */
  protected override(i: ResolvedHeader, value: string, event?: Event): void {
    this.add({ key: i.key, value }, 'value', event);
  }

  /** The checkbox of an inherited row: unchecked ignores it here, checked sets it back. */
  protected toggleInherited(i: ResolvedHeader): void {
    this.add(i.ignored ? { key: i.key, value: i.rawValue } : { key: i.key, ignore: true });
  }

  /** The checkbox of a local row: an ignore row goes away (the inherited header is back); others toggle. */
  protected toggleLocal(row: MergedRow<ResolvedHeader>): void {
    const r = this.local(row);
    if (r.ignore && row.inherited) {
      this.remove(row.local!);
    } else {
      this.set(row.local!, { enabled: !r.enabled });
    }
  }
}
