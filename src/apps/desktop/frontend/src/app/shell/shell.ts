import { ChangeDetectionStrategy, Component, DestroyRef, inject, signal } from '@angular/core';
import { TranslocoDirective } from '@jsverse/transloco';
import { System, Window } from '@wailsio/runtime';
import { AppService } from '@bindings/apps/desktop';

/**
 * Main window: the top bar (it is the window's title bar, see main.go), the
 * sidebar (workspace, environment, search, tree) and the request tabs.
 */
@Component({
  selector: 'app-shell',
  imports: [TranslocoDirective],
  templateUrl: './shell.html',
  styleUrl: './shell.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class Shell {
  // On Windows and Linux the bar holds the window buttons; on macOS the
  // native traffic lights sit at its left.
  protected readonly isMac = System.IsMac();
  protected readonly maximised = signal(false);
  protected readonly version = signal('');

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
    if (!(event.target as HTMLElement).closest('button, a, select, input')) {
      this.toggleMaximise();
    }
  }
}
