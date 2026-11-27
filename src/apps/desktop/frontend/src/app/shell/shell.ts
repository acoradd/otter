import { ChangeDetectionStrategy, Component, DestroyRef, inject, signal } from '@angular/core';
import { TranslocoDirective, TranslocoService } from '@jsverse/transloco';
import { System, Window } from '@wailsio/runtime';
import { AppService } from '@bindings/apps/desktop';

import { errorKey } from '../core/error-key';
import { WorkspaceStore } from '../core/workspace.store';
import { Environments } from '../environments/environments';
import { SettingsMenu } from '../settings-menu/settings-menu';
import { NameForm } from '../shared/name-form';
import { Sidebar } from '../sidebar/sidebar';
import { FolderTab, RequestTab, Tab, TabsStore, isDirty, tabKey } from '../core/tabs.store';
import { FolderEditor } from '../editors/folder-editor';
import { RequestEditor } from '../editors/request-editor';
import { TabBar } from '../editors/tab-bar';

/**
 * Main window: the top bar (it is the window's title bar, see main.go), the
 * sidebar (workspace, environment, tree) and the content.
 */
@Component({
  selector: 'app-shell',
  imports: [
    TranslocoDirective,
    Sidebar,
    Environments,
    SettingsMenu,
    NameForm,
    TabBar,
    RequestEditor,
    FolderEditor,
  ],
  templateUrl: './shell.html',
  styleUrl: './shell.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { '(document:keydown)': 'onKeydown($event)' },
})
export class Shell {
  protected readonly store = inject(WorkspaceStore);
  protected readonly tabs = inject(TabsStore);
  private readonly transloco = inject(TranslocoService);

  // On Windows and Linux the bar holds the window buttons; on macOS the
  // native traffic lights sit at its left.
  protected readonly isMac = System.IsMac();
  protected readonly maximised = signal(false);
  protected readonly version = signal('');
  /** What the content shows next to the sidebar. */
  protected readonly panel = signal<'home' | 'environments'>('home');
  protected readonly creating = signal(false);
  /** The tab with unsaved changes the user asked to close. */
  protected readonly closing = signal<Tab | null>(null);
  protected readonly asRequest = (t: Tab) => t as RequestTab;
  protected readonly asFolder = (t: Tab) => t as FolderTab;

  constructor() {
    const refreshMaximised = () => {
      void Window.IsMaximised().then(
        (m) => this.maximised.set(m),
        () => undefined,
      );
    };
    refreshMaximised();
    window.addEventListener('resize', refreshMaximised);
    inject(DestroyRef).onDestroy(() => window.removeEventListener('resize', refreshMaximised));

    void AppService.GetAppInfo().then((info) => this.version.set(info?.version ?? ''));
    void this.store.refresh();
  }

  protected async createWorkspace(name: string): Promise<void> {
    if (await this.store.create(name)) {
      this.creating.set(false);
    }
  }

  /** Ctrl+S saves the active editor, Ctrl+W closes it (Cmd on macOS). */
  protected onKeydown(event: KeyboardEvent): void {
    if (!(event.ctrlKey || event.metaKey) || event.altKey || event.shiftKey) {
      return;
    }
    const key = event.key.toLowerCase();
    const active = this.tabs.active();
    if (key === 's') {
      event.preventDefault();
      if (active) {
        void this.tabs.save(tabKey(active));
      }
    } else if (key === 'w') {
      event.preventDefault();
      if (active) {
        this.requestClose(tabKey(active));
      }
    }
  }

  /** Closes a tab, asking first when it has unsaved changes. */
  protected requestClose(key: string): void {
    const tab = this.tabs.tabs().find((t) => tabKey(t) === key);
    if (tab && isDirty(tab)) {
      this.closing.set(tab);
    } else {
      this.tabs.close(key);
    }
  }

  protected async closeSaving(): Promise<void> {
    const tab = this.closing();
    this.closing.set(null);
    if (tab && (await this.tabs.save(tabKey(tab)))) {
      this.tabs.close(tabKey(tab));
    }
  }

  protected closeDiscarding(): void {
    const tab = this.closing();
    this.closing.set(null);
    if (tab) {
      this.tabs.close(tabKey(tab));
    }
  }

  protected errorKey(code: string): string {
    return errorKey(this.transloco, code);
  }

  protected minimise(): void {
    void Window.Minimise();
  }

  protected toggleMaximise(): void {
    void Window.ToggleMaximise();
  }

  protected close(): void {
    void Window.Close();
  }

  /** Double-click on the bar, outside its controls, maximises or restores the window. */
  protected onTitleDoubleClick(event: MouseEvent): void {
    if (!(event.target as HTMLElement).closest('button, a, select, input, [role="dialog"]')) {
      this.toggleMaximise();
    }
  }
}
