import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';
import { TranslocoDirective } from '@jsverse/transloco';
import { BodyFieldRow, EditorService } from '@bindings/apps/desktop';

/**
 * The fields of a form URL-encoded or multipart body. In multipart, a field
 * is text or a file (a path read when sending), with an optional content type.
 */
@Component({
  selector: 'app-body-fields',
  imports: [TranslocoDirective],
  template: `
    <ng-container *transloco="let t">
      <table class="grid-table">
        <thead>
          <tr>
            <th class="col-check">
              <span class="sr-only">{{ t('editor.enabled') }}</span>
            </th>
            <th>{{ t('editor.fieldName') }}</th>
            @if (multipart()) {
              <th class="col-kind">{{ t('editor.fieldKind') }}</th>
            }
            <th>{{ t('editor.value') }}</th>
            @if (multipart()) {
              <th class="col-type">{{ t('editor.contentType') }}</th>
            }
            <th class="col-actions">
              <span class="sr-only">{{ t('common.actions') }}</span>
            </th>
          </tr>
        </thead>
        <tbody>
          @for (row of rows(); track $index) {
            <tr [class.disabled]="!row.enabled">
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
                  [attr.aria-label]="t('editor.fieldName')"
                  (input)="set($index, { key: $any($event.target).value })"
                />
              </td>
              @if (multipart()) {
                <td class="col-kind">
                  <select
                    class="input"
                    [attr.aria-label]="t('editor.fieldKind')"
                    (change)="set($index, { file: $any($event.target).value === 'file' })"
                  >
                    <option value="text" [selected]="!row.file">{{ t('editor.kindText') }}</option>
                    <option value="file" [selected]="row.file">{{ t('editor.kindFile') }}</option>
                  </select>
                </td>
              }
              <td>
                <div class="value">
                  <input
                    class="input mono"
                    [value]="row.value"
                    [attr.aria-label]="row.file ? t('editor.filePath') : t('editor.value')"
                    [placeholder]="row.file ? t('editor.filePath') : ''"
                    (input)="set($index, { value: $any($event.target).value })"
                  />
                  @if (row.file) {
                    <button type="button" class="btn" (click)="choose($index)">
                      {{ t('editor.chooseFile') }}
                    </button>
                  }
                </div>
              </td>
              @if (multipart()) {
                <td class="col-type">
                  <input
                    class="input mono"
                    [value]="row.contentType"
                    [placeholder]="t('editor.auto')"
                    [attr.aria-label]="t('editor.contentType')"
                    (input)="set($index, { contentType: $any($event.target).value })"
                  />
                </td>
              }
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
              <td [attr.colspan]="multipart() ? 6 : 4" class="empty">{{ t('editor.noFields') }}</td>
            </tr>
          }
        </tbody>
      </table>
      <button type="button" class="btn" (click)="add()">{{ t('editor.addField') }}</button>
    </ng-container>
  `,
  styles: `
    :host {
      display: flex;
      flex-direction: column;
      align-items: flex-start;
      gap: var(--space-2);
    }
    .col-kind {
      width: 96px;
    }
    .col-type {
      width: 160px;
    }
    .value {
      display: flex;
      gap: var(--space-1);
    }
    .value .input {
      flex: 1;
      min-width: 0;
    }
  `,
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class BodyFields {
  readonly rows = input.required<BodyFieldRow[]>();
  readonly multipart = input(false);
  readonly rowsChange = output<BodyFieldRow[]>();

  protected set(index: number, change: Partial<BodyFieldRow>): void {
    this.rowsChange.emit(this.rows().map((r, i) => (i === index ? { ...r, ...change } : r)));
  }

  protected remove(index: number): void {
    this.rowsChange.emit(this.rows().filter((_, i) => i !== index));
  }

  protected add(): void {
    this.rowsChange.emit([
      ...this.rows(),
      { key: '', file: false, value: '', contentType: '', enabled: true },
    ]);
  }

  protected async choose(index: number): Promise<void> {
    const path = await EditorService.ChooseFile(this.rows()[index].value);
    if (path) {
      this.set(index, { value: path });
    }
  }
}
