import { TestBed } from '@angular/core/testing';
import { provideTransloco } from '@jsverse/transloco';

import { demoBackend, fixtures } from '../../testing/workspace';
import { LanguageService, systemLanguage } from './language.service';

describe('LanguageService', () => {
  beforeEach(() => {
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        provideTransloco({ config: { availableLangs: ['en', 'fr'], defaultLang: 'en' } }),
      ],
    });
  });

  it('starts in the language stored by Go', async () => {
    demoBackend().on('SettingsService.GetSettings', { ...fixtures.settings(), language: 'fr' });
    const language = TestBed.inject(LanguageService);
    await language.init();
    expect(language.active).toBe('fr');
    expect(document.documentElement.lang).toBe('fr');
  });

  it('follows the language Go broadcasts', async () => {
    const backend = demoBackend();
    const language = TestBed.inject(LanguageService);
    await language.init();

    backend.emit('settings.changed', { ...fixtures.settings(), language: 'fr' });
    TestBed.tick();
    expect(language.active).toBe('fr');
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
