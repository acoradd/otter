import { ElementRef, afterEveryRender, inject } from '@angular/core';

/**
 * Keeps the caret in a field that moves: typing in an inherited row creates
 * a local one, and a local row can move next to the inherited row it
 * replaces. The field is found again by its local index and column.
 */
export class FocusFollow {
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private pending: { local: number; col: string; caret: number } | null = null;

  constructor() {
    afterEveryRender(() => this.restore());
  }

  /** Call when an edit of the field at (local, col) is about to re-render the list. */
  follow(local: number, col: string, event?: Event): void {
    const target = event?.target as HTMLInputElement | undefined;
    this.pending = { local, col, caret: target?.selectionStart ?? Number.MAX_SAFE_INTEGER };
  }

  private restore(): void {
    const p = this.pending;
    if (!p) {
      return;
    }
    this.pending = null;
    const el = this.host.nativeElement.querySelector<HTMLElement>(
      `[data-local="${p.local}"][data-col="${p.col}"]`,
    );
    const field = el?.matches('input')
      ? (el as HTMLInputElement)
      : el?.querySelector<HTMLInputElement>('input');
    if (field && document.activeElement !== field) {
      field.focus();
      const at = Math.min(p.caret, field.value.length);
      field.setSelectionRange(at, at);
    }
  }
}
