import { TestBed } from '@angular/core/testing';

import { click, find, render, settle } from '../../testing/render';
import { demoBackend, fixtures } from '../../testing/workspace';
import { AppSettings } from '../core/settings.store';
import { SettingsMenu } from './settings-menu';

describe('SettingsMenu', () => {
  afterEach(() => delete document.documentElement.dataset['theme']);

  it('changes the language and the theme through Go', async () => {
    const backend = demoBackend();
    const fixture = await render(SettingsMenu);
    await TestBed.inject(AppSettings).load();

    find(fixture, 'button[aria-label="Settings"]').click();
    await settle(fixture);
    expect(find(fixture, '.seg button.active').textContent?.trim()).toBe('System');

    await click(fixture, 'Français');
    await click(fixture, 'Dark');
    expect(backend.argsOf('SettingsService.SetLanguage')).toEqual([['fr']]);
    expect(backend.argsOf('SettingsService.SetTheme')).toEqual([['dark']]);
  });

  it('applies the theme Go broadcasts', async () => {
    const backend = demoBackend();
    const fixture = await render(SettingsMenu);
    await TestBed.inject(AppSettings).load();

    backend.emit('settings.changed', { ...fixtures.settings(), theme: 'light' });
    await settle(fixture);
    expect(document.documentElement.dataset['theme']).toBe('light');

    backend.emit('settings.changed', { ...fixtures.settings(), theme: 'system' });
    await settle(fixture);
    expect(document.documentElement.dataset['theme']).toBeUndefined();
  });
});
