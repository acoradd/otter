import { ChangeDetectionStrategy, Component, computed, input, output, signal } from '@angular/core';
import { TranslocoDirective } from '@jsverse/transloco';
import { FolderChoice, NodeKind, TreeNode, TreeService } from '@bindings/apps/desktop';

import { ROOT } from '../core/tree.store';

/** "Move to": picks the folder a node goes into, filtered by path. */
@Component({
  selector: 'app-move-dialog',
  imports: [TranslocoDirective],
  template: `
    <ng-container *transloco="let t">
      <div class="backdrop" (click)="cancelled.emit()"></div>
      <div
        class="dialog move"
        role="dialog"
        [attr.aria-label]="t('tree.moveTitle', { name: node().name })"
      >
        <h2>{{ t('tree.moveTitle', { name: node().name }) }}</h2>
        <input
          class="input"
          [placeholder]="t('tree.moveFilter')"
          [attr.aria-label]="t('tree.moveFilter')"
          (input)="filter.set($any($event.target).value)"
          (keydown.escape)="cancelled.emit()"
        />
        <ul class="choices" role="listbox">
          @if (isFolder()) {
            <li>
              <button type="button" role="option" (click)="chosen.emit(root)">
                {{ t('tree.root') }}
              </button>
            </li>
          }
          @for (c of shown(); track c.id) {
            <li>
              <button
                type="button"
                role="option"
                [attr.title]="c.label"
                (click)="chosen.emit(c.id)"
              >
                {{ c.label }}
              </button>
            </li>
          } @empty {
            <li class="empty">{{ t('tree.noMatch') }}</li>
          }
        </ul>
        <div class="dialog-actions">
          <button type="button" class="btn" (click)="cancelled.emit()">
            {{ t('common.cancel') }}
          </button>
        </div>
      </div>
    </ng-container>
  `,
  styles: `
    .move {
      width: 440px;
    }
    h2 {
      margin: 0;
      font-size: var(--text-lg);
    }
    .choices {
      max-height: 300px;
      margin: 0;
      padding: 0;
      overflow: auto;
      list-style: none;
    }
    .choices button {
      display: block;
      width: 100%;
      padding: var(--space-1) var(--space-2);
      overflow: hidden;
      border: 0;
      border-radius: var(--radius);
      background: transparent;
      text-align: left;
      text-overflow: ellipsis;
      white-space: nowrap;
      cursor: pointer;
    }
    .choices button:hover {
      background: var(--color-hover);
    }
  `,
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { '(document:keydown.escape)': 'cancelled.emit()' },
})
export class MoveDialog {
  readonly node = input.required<TreeNode>();
  readonly workspaceId = input.required<number>();
  readonly chosen = output<number>();
  readonly cancelled = output<void>();

  protected readonly root = ROOT;
  protected readonly filter = signal('');
  private readonly choices = signal<FolderChoice[]>([]);
  protected readonly isFolder = computed(() => this.node().kind === NodeKind.NodeFolder);
  protected readonly shown = computed(() => {
    const q = this.filter().trim().toLowerCase();
    const self = this.node();
    return this.choices()
      .filter((c) => !(self.kind === NodeKind.NodeFolder && c.id === self.id))
      .map((c) => ({ id: c.id, label: (c.path ?? []).join(' › ') }))
      .filter((c) => !q || c.label.toLowerCase().includes(q));
  });

  ngOnInit(): void {
    void TreeService.GetFolderChoices(this.workspaceId()).then((list) =>
      this.choices.set(list ?? []),
    );
  }
}
