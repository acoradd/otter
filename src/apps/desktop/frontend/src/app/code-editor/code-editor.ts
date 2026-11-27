import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  ElementRef,
  InjectionToken,
  afterNextRender,
  effect,
  inject,
  input,
  output,
  signal,
  untracked,
  viewChild,
} from '@angular/core';
import type * as Monaco from 'monaco-editor';

import { AppSettings } from '../core/settings.store';

export type CodeLanguage = 'json' | 'xml' | 'plaintext' | 'javascript';

/**
 * Loads Monaco on demand. Tests replace it: jsdom cannot run Monaco, and the
 * editor then stays a plain text area, as it does if Monaco fails to load.
 */
export const MONACO_LOADER = new InjectionToken<() => Promise<typeof Monaco>>('MONACO_LOADER', {
  providedIn: 'root',
  factory: () => () => import('./monaco').then((m) => m.load()),
});

/** Monaco, wrapped: value in, valueChange out, theme following the app's. */
@Component({
  selector: 'app-code-editor',
  template: `
    <div #host class="host" [class.hidden]="!ready()"></div>
    @if (!ready()) {
      <textarea
        class="fallback"
        spellcheck="false"
        [value]="value()"
        [readOnly]="readOnly()"
        [attr.aria-label]="label()"
        (input)="valueChange.emit($any($event.target).value)"
      ></textarea>
    }
  `,
  styles: `
    :host {
      position: relative;
      display: block;
      text-align: left;
      min-height: 160px;
      border: 1px solid var(--color-border);
      border-radius: var(--radius);
      overflow: hidden;
    }
    .host,
    .fallback {
      position: absolute;
      inset: 0;
    }
    .hidden {
      visibility: hidden;
    }
    .fallback {
      width: 100%;
      height: 100%;
      padding: var(--space-2);
      border: 0;
      background: var(--color-bg);
      font-family: var(--font-mono);
      font-size: var(--text-sm);
      resize: none;
      outline: none;
    }
  `,
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class CodeEditor {
  readonly value = input('');
  readonly language = input<CodeLanguage>('plaintext');
  readonly readOnly = input(false);
  readonly label = input('');
  readonly valueChange = output<string>();

  protected readonly ready = signal(false);
  private readonly host = viewChild.required<ElementRef<HTMLElement>>('host');
  private readonly settings = inject(AppSettings);
  private readonly loader = inject(MONACO_LOADER);
  private monaco: typeof Monaco | null = null;
  private editor: Monaco.editor.IStandaloneCodeEditor | null = null;
  /** True while the editor's text is set from value(), not typed. */
  private setting = false;

  constructor() {
    const destroyRef = inject(DestroyRef);
    afterNextRender(() => {
      this.loader().then(
        (m) => {
          if (!destroyRef.destroyed) {
            this.create(m);
          }
        },
        () => undefined, // stays a text area
      );
    });
    destroyRef.onDestroy(() => {
      this.editor?.getModel()?.dispose();
      this.editor?.dispose();
    });

    effect(() => {
      const value = this.value();
      untracked(() => {
        if (this.editor && this.editor.getValue() !== value) {
          this.setting = true;
          this.editor.setValue(value);
          this.setting = false;
        }
      });
    });
    effect(() => {
      const language = this.language();
      untracked(() => {
        const model = this.editor?.getModel();
        if (model && this.monaco) {
          this.monaco.editor.setModelLanguage(model, language);
        }
      });
    });
    effect(() => {
      const readOnly = this.readOnly();
      untracked(() => this.editor?.updateOptions({ readOnly }));
    });
    // The theme follows the app's: forced in the settings, or the system's.
    const media = window.matchMedia?.('(prefers-color-scheme: dark)');
    const applyTheme = () => this.applyTheme();
    media?.addEventListener('change', applyTheme);
    destroyRef.onDestroy(() => media?.removeEventListener('change', applyTheme));
    effect(() => {
      this.settings.value();
      untracked(() => setTimeout(applyTheme)); // after styles.scss switched the tokens
    });
  }

  private create(m: typeof Monaco): void {
    this.monaco = m;
    this.editor = m.editor.create(this.host().nativeElement, {
      value: this.value(),
      language: this.language(),
      readOnly: this.readOnly(),
      automaticLayout: true,
      minimap: { enabled: false },
      scrollBeyondLastLine: false,
      fontFamily: getComputedStyle(document.documentElement).getPropertyValue('--font-mono'),
      fontSize: 12,
      tabSize: 2,
      ariaLabel: this.label(),
      fixedOverflowWidgets: true,
    });
    this.editor.onDidChangeModelContent(() => {
      if (!this.setting) {
        this.valueChange.emit(this.editor!.getValue());
      }
    });
    this.applyTheme();
    this.ready.set(true);
  }

  private applyTheme(): void {
    if (!this.monaco) {
      return;
    }
    void import('./monaco').then(({ defineThemes }) => {
      const forced = document.documentElement.dataset['theme'];
      const dark = forced
        ? forced === 'dark'
        : !!window.matchMedia?.('(prefers-color-scheme: dark)').matches;
      defineThemes(this.monaco!);
      this.monaco!.editor.setTheme(dark ? 'otter-dark' : 'otter-light');
    });
  }
}
