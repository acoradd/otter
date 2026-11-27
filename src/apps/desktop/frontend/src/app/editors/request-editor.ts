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
import { SendStore } from '../core/send.store';
import { ResponsePanel } from '../response/response-panel';
import { errorKey } from '../core/error-key';
import { PreviewStore, varMap } from '../core/preview.store';
import { RequestTab, TabsStore, isDirty, tabKey } from '../core/tabs.store';
import { VarInput } from '../shared/var-input';
import { BodyFields } from './body-fields';
import { MergedHeaders } from './merged-headers';
import { MergedVars } from './merged-vars';

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
  imports: [
    TranslocoDirective,
    MergedHeaders,
    MergedVars,
    BodyFields,
    CodeEditor,
    VarInput,
    ResponsePanel,
  ],
  host: { '[style.--split]': 'split() + "%"' },
  templateUrl: './request-editor.html',
  styleUrl: './editor.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class RequestEditor {
  readonly tab = input.required<RequestTab>();
  protected readonly tabs = inject(TabsStore);
  private readonly transloco = inject(TranslocoService);

  protected readonly section = signal<Section>('headers');
  private readonly sends = inject(SendStore);
  protected readonly sending = computed(() => this.sends.state(this.tab().id).sending);
  /** Height of the request part, in % of the editor; the response gets the rest. */
  protected readonly split = signal(55);
  protected readonly bodyTypes = BODY_TYPES;
  protected readonly draft = computed(() => this.tab().draft);
  protected readonly dirty = computed(() => isDirty(this.tab()));
  /** The standard methods, and the request's own when it is another one. */
  protected readonly methods = computed(() => {
    const m = this.draft().method;
    return METHODS.includes(m) ? METHODS : [...METHODS, m];
  });
  private readonly previews = inject(PreviewStore);
  protected readonly preview = computed(() => this.previews.of(this.tab()));
  protected readonly vars = computed(() => varMap(this.preview()));
  /** Headers sent and variables defined, and where undefined variables are used. */
  protected readonly counts = computed(() => {
    const p = this.preview();
    const missing = (p?.vars ?? []).filter((v) => !v.source);
    const usedIn = (prefixes: string[]) =>
      missing.filter((v) =>
        (v.usedIn ?? []).some((u) => prefixes.some((x) => u === x || u.startsWith(x + ':'))),
      ).length;
    return {
      headers: (p?.headers ?? []).filter((h) => !h.ignored).length,
      vars: (p?.vars ?? []).filter((v) => v.source).length,
      missing: missing.length,
      missingHeaders: usedIn(['header']),
      missingBody: usedIn(['body', 'field', 'file']),
    };
  });
  /** Warnings other than undefined variables (they are highlighted where used). */
  protected readonly warnings = computed(() =>
    (this.preview()?.warnings ?? []).filter((w) => w.code !== 'UNRESOLVED_VAR'),
  );
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

  send(): void {
    void this.sends.send(this.tab().id, this.draft());
  }

  protected cancel(): void {
    this.sends.cancel(this.tab().id);
  }

  /** Drags the splitter between the request and its response (pointer events). */
  protected startResize(event: PointerEvent): void {
    const host = (event.target as HTMLElement).parentElement!;
    const box = host.getBoundingClientRect();
    const move = (e: PointerEvent) => {
      if (box.height > 0) {
        this.split.set(Math.min(85, Math.max(20, ((e.clientY - box.top) / box.height) * 100)));
      }
    };
    const up = () => {
      document.removeEventListener('pointermove', move);
      document.removeEventListener('pointerup', up);
    };
    document.addEventListener('pointermove', move);
    document.addEventListener('pointerup', up);
    event.preventDefault();
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
