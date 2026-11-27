import { DestroyRef, Injectable, inject, signal } from '@angular/core';
import { Events } from '@wailsio/runtime';
import { Settings, SettingsService } from '@bindings/apps/desktop';

import { BackendError, backendError } from './backend-error';

export type Theme = 'system' | 'light' | 'dark';

/** The app's preferences, stored by Go; Go broadcasts settings.changed after each change. */
@Injectable({ providedIn: 'root' })
export class AppSettings {
  readonly value = signal<Settings | null>(null);
  readonly error = signal<BackendError | null>(null);

  constructor() {
    const off = Events.On('settings.changed', (event) => this.apply(event.data));
    inject(DestroyRef).onDestroy(off);
  }

  /** Loads the settings once at start. */
  async load(): Promise<Settings | null> {
    try {
      this.apply(await SettingsService.GetSettings());
    } catch (err) {
      this.error.set(backendError(err));
    }
    return this.value();
  }

  setLanguage(language: string): Promise<boolean> {
    return this.run(() => SettingsService.SetLanguage(language));
  }

  setTheme(theme: Theme): Promise<boolean> {
    return this.run(() => SettingsService.SetTheme(theme));
  }

  /** Runs a setter; the new settings arrive with settings.changed. */
  private async run(call: () => Promise<unknown>): Promise<boolean> {
    try {
      await call();
      this.error.set(null);
      return true;
    } catch (err) {
      this.error.set(backendError(err));
      return false;
    }
  }

  private apply(settings: Settings): void {
    this.value.set(settings);
    applyTheme(settings.theme as Theme);
  }
}

/** Forces a theme on the root element, or lets the system decide (styles.scss). */
export function applyTheme(theme: Theme): void {
  const root = document.documentElement;
  if (theme === 'light' || theme === 'dark') {
    root.dataset['theme'] = theme;
  } else {
    delete root.dataset['theme'];
  }
}
