import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';
import { TranslocoDirective } from '@jsverse/transloco';

/** A row of a key/value table. ignore: a header removed rather than set. */
export interface KvRow {
  key: string;
  value: string;
  enabled: boolean;
  ignore?: boolean;
}

/**
 * An editable list of name/value rows (headers, variables), each with its
 * checkbox. A trailing button adds a row; blank rows are dropped by Go.
 */
@Component({
  selector: 'app-kv-table',
  imports: [TranslocoDirective],
  template: `
    <table class="grid-table" *transloco="let t">
      <thead>
        <tr>
          <th class="col-check">
            <span class="sr-only">{{ t('editor.enabled') }}</span>
          </th>
          <th>{{ keyLabel() }}</th>
          <th>{{ t('editor.value') }}</th>
          <th class="col-actions">
            <span class="sr-only">{{ t('common.actions') }}</span>
          </th>
        </tr>
      </thead>
      <tbody>
        @for (row of rows(); track $index) {
          <tr [class.disabled]="!row.enabled" [class.ignored]="row.ignore">
            <td class="col-check">
              <input
                type="checkbox"
                [checked]="row.enabled"
                [attr.aria-label]="t('editor.enabled')"
                (change)="set($index, { enabled: $any($event.target).checked })"
              />
            </td>
            <td>
              <input
                class="input mono"
                [value]="row.key"
                [attr.aria-label]="keyLabel()"
                (input)="set($index, { key: $any($event.target).value })"
              />
            </td>
            <td>
              @if (row.ignore) {
                <span class="pill muted" [attr.title]="t('editor.ignoredHint')">{{
                  t('editor.ignored')
                }}</span>
              } @else {
                <input
                  class="input mono"
                  [value]="row.value"
                  [attr.aria-label]="t('editor.value')"
                  (input)="set($index, { value: $any($event.target).value })"
                />
              }
            </td>
            <td class="col-actions">
              <button
                type="button"
                class="icon-btn"
                [attr.aria-label]="t('editor.removeRow')"
                [attr.title]="t('editor.removeRow')"
                (click)="remove($index)"
              >
                <svg viewBox="0 0 12 12" aria-hidden="true"><path d="M2 2l8 8M10 2l-8 8" /></svg>
              </button>
            </td>
          </tr>
        } @empty {
          <tr>
            <td colspan="4" class="empty">{{ emptyLabel() }}</td>
          </tr>
        }
      </tbody>
    </table>
    <button type="button" class="btn add" *transloco="let t" (click)="add()">
      {{ addLabel() }}
    </button>
  `,
  styles: `
    :host {
      display: flex;
      flex-direction: column;
      align-items: flex-start;
      gap: var(--space-2);
    }
    .ignored .input {
      text-decoration: line-through;
    }
  `,
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class KvTable<R extends KvRow> {
  readonly rows = input.required<R[]>();
  readonly keyLabel = input.required<string>();
  readonly addLabel = input.required<string>();
  readonly emptyLabel = input.required<string>();
  readonly rowsChange = output<R[]>();

  protected set(index: number, change: Partial<KvRow>): void {
    this.rowsChange.emit(this.rows().map((r, i) => (i === index ? { ...r, ...change } : r)));
  }

  protected remove(index: number): void {
    this.rowsChange.emit(this.rows().filter((_, i) => i !== index));
  }

  protected add(): void {
    this.rowsChange.emit([...this.rows(), { key: '', value: '', enabled: true } as R]);
  }
}
