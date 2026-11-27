import { ChangeDetectionStrategy, Component, inject, output } from '@angular/core';
import { TranslocoDirective } from '@jsverse/transloco';
import { RequestDraft } from '@bindings/apps/desktop';

import { Tab, TabsStore, isDirty, tabKey } from '../core/tabs.store';

/** The open editors. A dot marks unsaved changes; middle-click closes. */
@Component({
  selector: 'app-tab-bar',
  imports: [TranslocoDirective],
  template: `
    <div class="bar" role="tablist" *transloco="let t">
      @for (tab of tabs.tabs(); track key(tab)) {
        <div
          class="tab"
          role="tab"
          [class.active]="key(tab) === tabs.activeKey()"
          [attr.aria-selected]="key(tab) === tabs.activeKey()"
          [attr.title]="[...tab.path, tab.name].join(' › ')"
          (click)="tabs.activate(key(tab))"
          (auxclick)="$event.button === 1 && closeRequested.emit(key(tab))"
        >
          @if (tab.kind === 'request') {
            <span [class]="'method method-' + method(tab).toLowerCase()">{{ method(tab) }}</span>
          } @else {
            <svg class="folder" viewBox="0 0 12 12" aria-hidden="true">
              <path d="M1 3.5V10h10V4.5H5.5l-1-1.5H1z" />
            </svg>
          }
          <span class="name">{{ tab.name }}</span>
          <button
            type="button"
            class="close"
            [class.dirty]="dirty(tab)"
            [attr.aria-label]="
              t(dirty(tab) ? 'tabs.closeUnsaved' : 'tabs.close', { name: tab.name })
            "
            [attr.title]="t('tabs.closeShortcut')"
            (click)="$event.stopPropagation(); closeRequested.emit(key(tab))"
          >
            <svg viewBox="0 0 12 12" aria-hidden="true"><path d="M3 3l6 6M9 3l-6 6" /></svg>
          </button>
        </div>
      }
    </div>
  `,
  styles: `
    .bar {
      display: flex;
      overflow-x: auto;
      border-bottom: 1px solid var(--color-border);
      background: var(--color-surface);
    }
    .tab {
      display: flex;
      flex: none;
      align-items: center;
      gap: 6px;
      max-width: 240px;
      height: 34px;
      padding: 0 var(--space-1) 0 var(--space-3);
      border-right: 1px solid var(--color-border);
      border-bottom: 2px solid transparent;
      color: var(--color-text-muted);
      cursor: default;
      user-select: none;
    }
    .tab.active {
      border-bottom-color: var(--color-accent);
      background: var(--color-bg);
      color: var(--color-text);
    }
    .method {
      font-family: var(--font-mono);
      font-size: 10px;
      font-weight: 600;
    }
    .folder {
      width: 12px;
      height: 12px;
      fill: none;
      stroke: currentColor;
      stroke-width: 1.2;
    }
    .name {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .close {
      display: grid;
      place-items: center;
      width: 20px;
      height: 20px;
      padding: 0;
      border: 0;
      border-radius: 4px;
      background: transparent;
      color: inherit;
      cursor: pointer;
    }
    .close svg {
      width: 10px;
      height: 10px;
      fill: none;
      stroke: currentColor;
      stroke-width: 1.4;
    }
    .close:hover {
      background: var(--color-hover);
    }
    /* Unsaved: a dot, the cross on hover. */
    .close.dirty:not(:hover) svg {
      display: none;
    }
    .close.dirty:not(:hover)::after {
      content: '';
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background: var(--color-text-muted);
    }
  `,
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class TabBar {
  protected readonly tabs = inject(TabsStore);
  readonly closeRequested = output<string>();

  protected readonly key = tabKey;
  protected readonly dirty = isDirty;
  protected method(tab: Tab): string {
    return (tab.draft as RequestDraft).method;
  }
}
