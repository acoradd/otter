import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { TranslocoDirective } from '@jsverse/transloco';

import { AppSettings, Theme } from '../core/settings.store';

/** The settings button of the top bar and its menu: language and theme. */
@Component({
  selector: 'app-settings-menu',
  imports: [TranslocoDirective],
  templateUrl: './settings-menu.html',
  styleUrl: './settings-menu.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { '(document:keydown.escape)': 'open.set(false)' },
})
export class SettingsMenu {
  protected readonly settings = inject(AppSettings);
  protected readonly open = signal(false);

  protected readonly languages = ['', 'en', 'fr'];
  protected readonly themes: Theme[] = ['system', 'light', 'dark'];
  protected readonly language = computed(() => this.settings.value()?.language ?? '');
  protected readonly theme = computed(() => this.settings.value()?.theme ?? 'system');

  protected setLanguage(language: string): void {
    void this.settings.setLanguage(language);
  }

  protected setTheme(theme: Theme): void {
    void this.settings.setTheme(theme);
  }
}
