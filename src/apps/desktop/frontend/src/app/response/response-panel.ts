import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  input,
  signal,
  untracked,
} from '@angular/core';
import { TranslocoDirective, TranslocoService } from '@jsverse/transloco';

import { CodeEditor, CodeLanguage } from '../code-editor/code-editor';
import { errorKey } from '../core/error-key';
import { formatBytes, formatDateTime, formatDuration, prettyJson } from '../core/format';
import { bodyFormat } from '../core/body-format';
import { prettyMarkup } from '../core/markup';
import { SendStore } from '../core/send.store';

type Section = 'body' | 'headers' | 'sent' | 'redirects' | 'history';

/** The response of a request: status, timing, size, then body, headers, what was sent, redirects, history. */
@Component({
  selector: 'app-response-panel',
  imports: [TranslocoDirective, CodeEditor],
  templateUrl: './response-panel.html',
  styleUrl: './response-panel.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ResponsePanel {
  readonly requestId = input.required<number>();
  protected readonly sends = inject(SendStore);
  private readonly transloco = inject(TranslocoService);

  protected readonly section = signal<Section>('body');
  protected readonly pretty = signal(true);
  protected readonly saved = signal('');
  protected readonly state = computed(() => this.sends.state(this.requestId()));
  protected readonly response = computed(() => this.state().response);
  protected readonly statusClass = computed(() => this.classOf(this.response()?.status ?? 0));
  /** The format: from the Content-Type, else recognized in the content. */
  protected readonly format = computed(() => {
    const r = this.response();
    return bodyFormat(r?.contentType ?? '', r?.bodyKind === 'text' ? r.body : '');
  });
  protected readonly language = computed<CodeLanguage>(() => this.format().format);
  /** JSON, XML and HTML can be shown indented. */
  protected readonly formattable = computed(() => this.language() !== 'plaintext');
  protected readonly body = computed(() => {
    const r = this.response();
    if (!r || r.bodyKind !== 'text') {
      return '';
    }
    if (!this.pretty()) {
      return r.body;
    }
    switch (this.language()) {
      case 'json':
        return prettyJson(r.body);
      case 'html':
        return prettyMarkup(r.body, true);
      case 'xml':
        return prettyMarkup(r.body, false);
    }
    return r.body;
  });

  constructor() {
    effect(() => {
      const id = this.requestId();
      untracked(() => {
        this.saved.set('');
        void this.sends.loadHistory(id);
      });
    });
  }

  /** s2 for 2xx, s4 for 4xx… */
  protected classOf(status: number): string {
    return status >= 500
      ? 's5'
      : status >= 400
        ? 's4'
        : status >= 300
          ? 's3'
          : status >= 200
            ? 's2'
            : 's1';
  }

  protected lang(): string {
    return this.transloco.getActiveLang();
  }

  protected bytes(n: number): string {
    return formatBytes(n, this.lang());
  }

  protected duration(ms: number): string {
    return formatDuration(ms, this.lang());
  }

  protected dateTime(ms: number): string {
    return formatDateTime(ms, this.lang());
  }

  protected errorKey(code: string): string {
    return errorKey(this.transloco, code);
  }

  protected async save(): Promise<void> {
    const r = this.response();
    if (r) {
      this.saved.set(await this.sends.saveBody(this.requestId(), r.id));
    }
  }

  protected show(entryId: number): void {
    this.saved.set('');
    this.section.set('body');
    void this.sends.showHistory(this.requestId(), entryId);
  }
}
