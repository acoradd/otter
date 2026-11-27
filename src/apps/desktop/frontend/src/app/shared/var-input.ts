import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  computed,
  effect,
  inject,
  input,
  output,
  signal,
  untracked,
  viewChild,
} from '@angular/core';
import { TranslocoService } from '@jsverse/transloco';

import { VarMap } from '../core/preview.store';
import { Level, describeVar } from '../core/source-label';

/** {{name}}, as Go reads it. */
const PLACEHOLDER = /\{\{\s*([\w.-]+)\s*\}\}/g;

interface Segment {
  text: string;
  /** The variable of a {{name}} segment. */
  name?: string;
  missing?: boolean;
  start: number;
}

/** Splits a text into plain parts and {{name}} parts. */
export function segments(text: string, vars: VarMap): Segment[] {
  const out: Segment[] = [];
  let last = 0;
  for (const m of text.matchAll(PLACEHOLDER)) {
    if (m.index > last) {
      out.push({ text: text.slice(last, m.index), start: last });
    }
    out.push({ text: m[0], name: m[1], missing: !vars.get(m[1])?.source, start: m.index });
    last = m.index + m[0].length;
  }
  if (last < text.length) {
    out.push({ text: text.slice(last), start: last });
  }
  return out;
}

/**
 * A one-line text field that highlights {{variables}} (red and wavy when
 * defined nowhere) and tells their value and source on hover. The text is
 * drawn by a mirror under a transparent input; both use the monospace font,
 * so the hovered character follows from the pointer's position.
 */
@Component({
  selector: 'app-var-input',
  template: `
    <div #mirror class="mirror" aria-hidden="true">
      @for (s of parts(); track s.start) {
        @if (s.name) {
          <span class="var" [class.missing]="s.missing">{{ s.text }}</span>
        } @else {
          <span>{{ s.text }}</span>
        }
      }
    </div>
    <input
      #field
      class="field"
      spellcheck="false"
      [value]="value()"
      [placeholder]="placeholder()"
      [attr.aria-label]="label()"
      [attr.aria-invalid]="hasMissing() || null"
      (input)="onInput($event)"
      (scroll)="sync()"
      (keyup)="sync()"
      (click)="sync()"
      (select)="sync()"
      (mousemove)="hover($event)"
      (mouseleave)="tip.set(null)"
      (paste)="pasted.emit($event)"
    />
    @if (tip(); as tp) {
      <div class="var-tip" role="tooltip" [style.left.px]="tp.x" [style.top.px]="tp.y">
        @for (line of tp.lines; track $index) {
          <div [class.head]="$first">{{ line }}</div>
        }
      </div>
    }
  `,
  styles: `
    :host {
      position: relative;
      display: block;
      min-width: 0;
      height: 28px;
      border: 1px solid var(--color-border);
      border-radius: var(--radius);
      background: var(--color-bg);
      font-family: var(--font-mono);
      font-size: var(--text-sm);
    }
    :host(:focus-within) {
      border-color: var(--color-accent);
    }
    .mirror,
    .field {
      position: absolute;
      inset: 0;
      padding: 0 var(--space-2);
      font: inherit;
      letter-spacing: 0;
      white-space: pre;
    }
    .mirror {
      display: flex;
      align-items: center;
      overflow: hidden;
      color: var(--color-text);
      pointer-events: none;
    }
    .field {
      width: 100%;
      border: 0;
      background: transparent;
      color: transparent;
      caret-color: var(--color-text);
      outline: none;
    }
    .field::placeholder {
      color: var(--color-text-muted);
      opacity: 0.7;
    }
    .field::selection {
      background: color-mix(in srgb, var(--color-accent) 35%, transparent);
      color: transparent;
    }
    .var {
      color: var(--color-accent);
    }
    .var.missing {
      color: var(--color-danger);
      text-decoration: underline wavy var(--color-danger);
      text-underline-offset: 3px;
    }
  `,
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class VarInput {
  readonly value = input('');
  readonly vars = input<VarMap>(new Map());
  readonly placeholder = input('');
  readonly label = input('');
  readonly level = input<Level>('request');
  readonly valueChange = output<string>();
  /** A paste in the field: the parent may take it over (preventDefault). */
  readonly pasted = output<ClipboardEvent>();

  private readonly transloco = inject(TranslocoService);
  private readonly field = viewChild.required<ElementRef<HTMLInputElement>>('field');
  private readonly mirror = viewChild.required<ElementRef<HTMLElement>>('mirror');

  /** The text as typed, ahead of value() until the parent echoes it back. */
  private readonly typed = signal<string | null>(null);
  protected readonly parts = computed(() => segments(this.typed() ?? this.value(), this.vars()));
  protected readonly hasMissing = computed(() => this.parts().some((p) => p.missing));
  protected readonly tip = signal<{ x: number; y: number; lines: string[] } | null>(null);

  constructor() {
    effect(() => {
      this.value();
      untracked(() => {
        this.typed.set(null);
        queueMicrotask(() => this.sync());
      });
    });
  }

  /** The text field, for focusing it from outside. */
  focus(caret?: number): void {
    const el = this.field().nativeElement;
    el.focus();
    const at = caret ?? el.value.length;
    el.setSelectionRange(at, at);
    this.sync();
  }

  protected onInput(event: Event): void {
    const text = (event.target as HTMLInputElement).value;
    this.typed.set(text);
    this.valueChange.emit(text);
    this.sync();
  }

  /** Keeps the mirror scrolled like the field. */
  protected sync(): void {
    this.mirror().nativeElement.scrollLeft = this.field().nativeElement.scrollLeft;
  }

  protected hover(event: MouseEvent): void {
    const el = this.field().nativeElement;
    const style = getComputedStyle(el);
    const box = el.getBoundingClientRect();
    const x =
      event.clientX - box.left - (Number.parseFloat(style.paddingLeft) || 0) + el.scrollLeft;
    const index = Math.floor(x / charWidth(style.font));
    const part = this.parts().find(
      (p) => p.name && index >= p.start && index < p.start + p.text.length,
    );
    if (!part?.name) {
      this.tip.set(null);
      return;
    }
    this.tip.set({
      x: event.clientX - box.left,
      y: box.height + 4,
      lines: describeVar(this.transloco, part.name, this.vars().get(part.name), this.level()),
    });
  }
}

const widths = new Map<string, number>();

/** The advance of one character of a monospace font, in px. */
function charWidth(font: string): number {
  let w = widths.get(font);
  if (w === undefined) {
    const ctx = document.createElement('canvas').getContext?.('2d');
    if (ctx) {
      ctx.font = font;
      w = ctx.measureText('0'.repeat(100)).width / 100;
    }
    w = w || 7.2;
    widths.set(font, w);
  }
  return w;
}
