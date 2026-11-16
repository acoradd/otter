import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';

import { FakeBackend } from '../testing/backend';
import { render, text } from '../testing/render';
import { App } from './app';
import { appConfig } from './app.config';
import { TranslocoHttpLoader } from './core/transloco-loader';

describe('App', () => {
  it('renders the main window', async () => {
    new FakeBackend();
    const fixture = await render(App);
    expect(text(fixture)).toContain('Welcome to Otter');
  });

  it('configures translations and the language at start', () => {
    expect(appConfig.providers.length).toBeGreaterThan(3);
  });

  it('loads the translations of a language over HTTP', () => {
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting()],
    });
    const loader = TestBed.inject(TranslocoHttpLoader);
    let got: unknown;
    loader.getTranslation('fr').subscribe((t) => (got = t));
    TestBed.inject(HttpTestingController)
      .expectOne('i18n/fr.json')
      .flush({ app: { name: 'Otter' } });
    expect(got).toEqual({ app: { name: 'Otter' } });
  });
});
