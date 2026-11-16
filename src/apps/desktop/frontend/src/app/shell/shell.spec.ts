import { AppInfo } from '@bindings/apps/desktop';

import { FakeBackend } from '../../testing/backend';
import appInfo from '../../testing/fixtures/app-info.json';
import { render, text } from '../../testing/render';
import { Shell } from './shell';

describe('Shell', () => {
  it('shows the empty state and the version from Go', async () => {
    const backend = new FakeBackend().on('AppService.GetAppInfo', appInfo as AppInfo);
    const fixture = await render(Shell);

    expect(backend.called('AppService.GetAppInfo')).toBe(true);
    expect(text(fixture)).toContain('Welcome to Otter');
    expect(text(fixture)).toContain('No workspace yet');
    expect(text(fixture)).toContain(`Version ${appInfo.version}`);
  });

  it('speaks French', async () => {
    new FakeBackend().on('AppService.GetAppInfo', appInfo as AppInfo);
    const fixture = await render(Shell, { lang: 'fr' });

    expect(text(fixture)).toContain('Bienvenue dans Otter');
  });

  it('draws its own window buttons outside macOS', async () => {
    new FakeBackend();
    const fixture = await render(Shell);
    const labels = [...(fixture.nativeElement as HTMLElement).querySelectorAll('.window-btn')].map(
      (b) => b.getAttribute('aria-label'),
    );

    expect(labels).toEqual(['Minimise', 'Maximise', 'Close']);
  });
});
