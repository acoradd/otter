import { ChangeDetectionStrategy, Component, computed, inject, input, signal } from '@angular/core';
import { TranslocoDirective, TranslocoService } from '@jsverse/transloco';
import {
  BodyFieldRow,
  EditorService,
  HeaderRow,
  RequestDraft,
  VarRow,
} from '@bindings/apps/desktop';

import { CodeEditor, CodeLanguage } from '../code-editor/code-editor';
import { errorKey } from '../core/error-key';
import { RequestTab, TabsStore, isDirty, tabKey } from '../core/tabs.store';
import { BodyFields } from './body-fields';
import { KvTable } from './kv-table';

export const METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS'];
export const BODY_TYPES = [
  'none',
  'json',
  'xml',
  'text',
  'form-urlencoded',
  'multipart',
  'binary',
] as const;
type Section = 'headers' | 'vars' | 'body' | 'script' | 'settings';

const LANGUAGES: Record<string, CodeLanguage> = { json: 'json', xml: 'xml', text: 'plaintext' };

/** The request editor: method and URL, then headers, variables, body, script and settings. */
@Component({
  selector: 'app-request-editor',
  imports: [TranslocoDirective, KvTable, BodyFields, CodeEditor],
  templateUrl: './request-editor.html',
  styleUrl: './editor.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class RequestEditor {
  readonly tab = input.required<RequestTab>();
  protected readonly tabs = inject(TabsStore);
  private readonly transloco = inject(TranslocoService);

  protected readonly section = signal<Section>('headers');
  protected readonly bodyTypes = BODY_TYPES;
  protected readonly draft = computed(() => this.tab().draft);
  protected readonly dirty = computed(() => isDirty(this.tab()));
  /** The standard methods, and the request's own when it is another one. */
  protected readonly methods = computed(() => {
    const m = this.draft().method;
    return METHODS.includes(m) ? METHODS : [...METHODS, m];
  });
  protected readonly counts = computed(() => {
    const d = this.draft();
    return {
      headers: (d.headers ?? []).filter((h) => h.enabled && h.key.trim()).length,
      vars: (d.vars ?? []).filter((v) => v.enabled && v.key.trim()).length,
    };
  });
  protected readonly language = computed(() => LANGUAGES[this.draft().bodyType] ?? 'plaintext');
  protected readonly urlPlaceholder = 'https://api.example.com/v1/users';

  protected edit(change: Partial<RequestDraft>): void {
    this.tabs.edit<RequestDraft>(tabKey(this.tab()), (d) => ({ ...d, ...change }));
  }

  protected setHeaders(headers: HeaderRow[]): void {
    this.edit({ headers });
  }

  protected setVars(vars: VarRow[]): void {
    this.edit({ vars });
  }

  protected setFields(bodyFields: BodyFieldRow[]): void {
    this.edit({ bodyFields });
  }

  /** Changing the body type keeps the other types' content; file fields only travel in multipart. */
  protected setBodyType(bodyType: string): void {
    const fields = this.draft().bodyFields ?? [];
    this.edit({
      bodyType,
      bodyFields:
        bodyType === 'multipart' ? fields : fields.map((f) => (f.file ? { ...f, file: false } : f)),
    });
  }

  protected async chooseBodyFile(): Promise<void> {
    const path = await EditorService.ChooseFile(this.draft().bodyFile);
    if (path) {
      this.edit({ bodyFile: path });
    }
  }

  protected number(value: string, fallback: number): number {
    const n = Number.parseInt(value, 10);
    return Number.isFinite(n) ? n : fallback;
  }

  protected save(): void {
    void this.tabs.save(tabKey(this.tab()));
  }

  protected revert(): void {
    this.tabs.revert(tabKey(this.tab()));
  }

  protected errorKey(code: string): string {
    return errorKey(this.transloco, code);
  }
}
