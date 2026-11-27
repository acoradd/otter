import { TestBed } from '@angular/core/testing';
import { TranslocoService, provideTransloco } from '@jsverse/transloco';
import { provideTranslocoMessageformat } from '@jsverse/transloco-messageformat';
import { SourceKind } from '@bindings/internal/resolve';

import en from '../../i18n/en.json';
import { describeVar, sourceLabel, usedInLabel } from './source-label';

describe('source labels', () => {
  let t: TranslocoService;
  beforeEach(() => {
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        provideTransloco({ config: { availableLangs: ['en'], defaultLang: 'en' } }),
        provideTranslocoMessageformat(),
      ],
    });
    t = TestBed.inject(TranslocoService);
    t.setTranslation(en, 'en');
    t.setActiveLang('en');
  });

  it('names every source, depth 0 being "here" only in a folder', () => {
    const folder = { kind: SourceKind.SourceFolder, path: ['api', 'users'], depth: 0 };
    expect(sourceLabel(t, folder)).toBe('Inherited from api › users');
    expect(sourceLabel(t, folder, 'folder')).toBe('Defined here');
    expect(sourceLabel(t, { ...folder, depth: 1 }, 'folder')).toBe('Inherited from api › users');
    expect(sourceLabel(t, { kind: SourceKind.SourceRequest })).toBe('Defined here');
    expect(sourceLabel(t, { kind: SourceKind.SourceEnv, name: 'dev' })).toBe('Environment dev');
    expect(sourceLabel(t, { kind: SourceKind.SourceRuntime })).toBe('Runtime variable');
    expect(sourceLabel(t, { kind: SourceKind.SourceAuto })).toBe('Automatic header');
    expect(sourceLabel(t, null)).toBe('Defined nowhere');
  });

  it('says where a variable is used, once each', () => {
    expect(
      usedInLabel(t, ['url', 'header:Authorization', 'field:name', 'field:name', 'body', 'file']),
    ).toBe('URL, header Authorization, field name, body, file path');
    expect(usedInLabel(t, null)).toBe('');
  });

  it('masks secrets, empty values and the hidden value', () => {
    const lines = describeVar(t, 'token', {
      key: 'token',
      value: 's3cr3t',
      rawValue: 's3cr3t',
      secret: true,
      source: { kind: SourceKind.SourceRequest },
      shadowed: [{ value: 'old', source: { kind: SourceKind.SourceEnv, name: 'dev' } }],
      usedIn: [],
    });
    expect(lines).toEqual(['{{token}} = ••••••', 'Defined here', 'Hides •••••• (Environment dev)']);
    expect(describeVar(t, 'x', undefined)).toEqual(['{{x}}', 'Not defined anywhere']);
  });
});
