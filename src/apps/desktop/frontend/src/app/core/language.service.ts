import { Injectable, effect, inject } from '@angular/core';
import { TranslocoService } from '@jsverse/transloco';

import { AppSettings } from './settings.store';

export const AVAILABLE_LANGUAGES = ['en', 'fr'] as const;
export type Language = (typeof AVAILABLE_LANGUAGES)[number];
export const DEFAULT_LANGUAGE: Language = 'en';

/**
 * UI language, switched at runtime. The choice is stored by Go (settings);
 * an empty choice follows the system language.
 */
@Injectable({ providedIn: 'root' })
export class LanguageService {
  private readonly transloco = inject(TranslocoService);
  private readonly settings = inject(AppSettings);

  readonly languages = AVAILABLE_LANGUAGES;

  constructor() {
    effect(() => this.set(this.settings.value()?.language || systemLanguage()));
  }

  async init(): Promise<void> {
    const settings = await this.settings.load();
    this.set(settings?.language || systemLanguage());
  }

  get active(): Language {
    return this.transloco.getActiveLang() as Language;
  }

  set(lang: string): void {
    if ((AVAILABLE_LANGUAGES as readonly string[]).includes(lang) && lang !== this.active) {
      this.transloco.setActiveLang(lang);
    }
    document.documentElement.lang = this.active;
  }
}

/** The system language when Otter speaks it, English otherwise. */
export function systemLanguage(language = navigator.language): Language {
  return language.toLowerCase().startsWith('fr') ? 'fr' : 'en';
}
