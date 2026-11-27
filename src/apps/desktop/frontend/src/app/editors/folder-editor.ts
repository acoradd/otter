import { ChangeDetectionStrategy, Component, computed, inject, input, signal } from '@angular/core';
import { TranslocoDirective, TranslocoService } from '@jsverse/transloco';
import { FolderDraft, HeaderRow, VarRow } from '@bindings/apps/desktop';

import { errorKey } from '../core/error-key';
import { PreviewStore } from '../core/preview.store';
import { FolderTab, TabsStore, isDirty, tabKey } from '../core/tabs.store';
import { MergedHeaders } from './merged-headers';
import { MergedVars } from './merged-vars';

/** The folder editor: the variables and headers its requests inherit. */
@Component({
  selector: 'app-folder-editor',
  imports: [TranslocoDirective, MergedHeaders, MergedVars],
  template: `
    <ng-container *transloco="let t">
      <div class="crumbs" [attr.title]="tab().path.join(' › ')">
        @for (p of parents(); track $index) {
          <span>{{ p }}</span><span class="sep">›</span>
        }
        <strong>{{ tab().name }}</strong>
      </div>
      <p class="hint">{{ t('editor.folderHint') }}</p>

      <nav class="subtabs" role="tablist">
        <button
          type="button"
          role="tab"
          [class.active]="section() === 'vars'"
          [attr.aria-selected]="section() === 'vars'"
          (click)="section.set('vars')"
        >
          {{ t('editor.variables') }}
          @if (counts().vars) {
            <span class="count">{{ counts().vars }}</span>
          }
        </button>
        <button
          type="button"
          role="tab"
          [class.active]="section() === 'headers'"
          [attr.aria-selected]="section() === 'headers'"
          (click)="section.set('headers')"
        >
          {{ t('editor.headers') }}
          @if (counts().headers) {
            <span class="count">{{ counts().headers }}</span>
          }
        </button>
      </nav>

      <section class="section" role="tabpanel">
        @if (section() === 'vars') {
          <app-merged-vars
            level="folder"
            [rows]="tab().draft.vars ?? []"
            [preview]="preview()"
            (rowsChange)="setVars($event)"
          />
        } @else {
          <app-merged-headers
            level="folder"
            [rows]="tab().draft.headers ?? []"
            [preview]="preview()"
            (rowsChange)="setHeaders($event)"
          />
        }
      </section>

      <footer class="footer">
        @if (tab().error; as err) {
          <div class="alert" role="alert">{{ t(errorKey(err.code), err.params ?? {}) }}</div>
        }
        <span class="spacer"></span>
        @if (dirty()) {
          <span class="unsaved">{{ t('common.unsaved') }}</span>
        }
        <button type="button" class="btn" [disabled]="!dirty()" (click)="tabs.revert(key())">
          {{ t('common.revert') }}
        </button>
        <button
          type="button"
          class="btn primary"
          [disabled]="!dirty()"
          [attr.title]="t('editor.saveShortcut')"
          (click)="save()"
        >
          {{ t('common.save') }}
        </button>
      </footer>
    </ng-container>
  `,
  styleUrl: './editor.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class FolderEditor {
  readonly tab = input.required<FolderTab>();
  protected readonly tabs = inject(TabsStore);
  private readonly previews = inject(PreviewStore);
  private readonly transloco = inject(TranslocoService);

  protected readonly section = signal<'vars' | 'headers'>('vars');
  protected readonly key = computed(() => tabKey(this.tab()));
  protected readonly dirty = computed(() => isDirty(this.tab()));
  protected readonly parents = computed(() => this.tab().path.slice(0, -1));
  protected readonly preview = computed(() => this.previews.of(this.tab()));
  /** Headers and variables that apply at this level, inherited ones included. */
  protected readonly counts = computed(() => {
    const p = this.preview();
    return {
      headers: (p?.headers ?? []).filter((h) => !h.ignored).length,
      vars: (p?.vars ?? []).filter((v) => v.source).length,
    };
  });

  protected setVars(vars: VarRow[]): void {
    this.tabs.edit<FolderDraft>(this.key(), (d) => ({ ...d, vars }));
  }

  protected setHeaders(headers: HeaderRow[]): void {
    this.tabs.edit<FolderDraft>(this.key(), (d) => ({ ...d, headers }));
  }

  protected save(): void {
    void this.tabs.save(this.key());
  }

  protected errorKey(code: string): string {
    return errorKey(this.transloco, code);
  }
}
