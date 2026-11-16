import { TestBed } from '@angular/core/testing';
import { provideTransloco } from '@jsverse/transloco';

import { LanguageService, systemLanguage } from './language.service';

describe('LanguageService', () => {
  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [
        provideTransloco({ config: { availableLangs: ['en', 'fr'], defaultLang: 'en' } }),
      ],
    });
  });

  it('switches to a known language and sets the document language', () => {
    const language = TestBed.inject(LanguageService);
    language.set('fr');
    expect(language.active).toBe('fr');
    expect(document.documentElement.lang).toBe('fr');
  });

  it('ignores an unknown language', () => {
    const language = TestBed.inject(LanguageService);
    language.set('de');
    expect(language.active).toBe('en');
  });

  it('follows the system language when Otter speaks it', () => {
    expect(systemLanguage('fr-FR')).toBe('fr');
    expect(systemLanguage('en-US')).toBe('en');
    expect(systemLanguage('de-DE')).toBe('en');
  });
});
