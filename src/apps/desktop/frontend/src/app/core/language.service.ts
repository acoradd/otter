import { Injectable, inject } from '@angular/core';
import { TranslocoService } from '@jsverse/transloco';

export const AVAILABLE_LANGUAGES = ['en', 'fr'] as const;
export type Language = (typeof AVAILABLE_LANGUAGES)[number];
export const DEFAULT_LANGUAGE: Language = 'en';

/**
 * UI language, switched at runtime. It follows the system language until the
 * choice is stored by Go in the setting table (broadcast with settings.changed).
 */
@Injectable({ providedIn: 'root' })
export class LanguageService {
  private readonly transloco = inject(TranslocoService);

  readonly languages = AVAILABLE_LANGUAGES;

  init(): void {
    this.set(systemLanguage());
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
