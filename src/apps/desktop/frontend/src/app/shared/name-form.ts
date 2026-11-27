import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  afterNextRender,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';
import { TranslocoDirective } from '@jsverse/transloco';

/** An inline form asking for a name: Enter submits, Escape cancels. */
@Component({
  selector: 'app-name-form',
  imports: [TranslocoDirective],
  template: `
    <form class="name-form" *transloco="let t" (submit)="submit($event)">
      <input
        #field
        class="input"
        [value]="value()"
        [attr.aria-label]="label()"
        [placeholder]="label()"
        (input)="name.set(field.value)"
        (keydown.escape)="cancelled.emit()"
      />
      <button type="submit" class="btn primary" [disabled]="!name().trim()">
        {{ submitLabel() }}
      </button>
      <button type="button" class="btn" (click)="cancelled.emit()">{{ t('common.cancel') }}</button>
    </form>
  `,
  styles: `
    .name-form {
      display: flex;
      gap: var(--space-1);
    }
    .input {
      flex: 1;
      min-width: 0;
    }
  `,
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class NameForm {
  readonly label = input.required<string>();
  readonly submitLabel = input.required<string>();
  readonly value = input('');
  readonly submitted = output<string>();
  readonly cancelled = output<void>();

  protected readonly name = signal('');
  private readonly field = viewChild.required<ElementRef<HTMLInputElement>>('field');

  constructor() {
    afterNextRender(() => {
      this.name.set(this.value());
      this.field().nativeElement.select();
    });
  }

  protected submit(event: Event): void {
    event.preventDefault();
    const name = this.name().trim();
    if (name) {
      this.submitted.emit(name);
    }
  }
}
