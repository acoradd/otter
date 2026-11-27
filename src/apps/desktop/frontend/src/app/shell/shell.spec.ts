import { TestBed } from '@angular/core/testing';

import { backendFailure } from '../../testing/backend';
import { buttons, click, find, render, settle, text, type } from '../../testing/render';
import { demoBackend, emptyBackend } from '../../testing/workspace';
import { AppSettings } from '../core/settings.store';
import { Shell } from './shell';

async function renderShell(lang: 'en' | 'fr' = 'en') {
  const fixture = await render(Shell, { lang });
  await TestBed.inject(AppSettings).load();
  await settle(fixture);
  return fixture;
}

describe('Shell', () => {
  it('opens the last workspace and shows the version from Go', async () => {
    const backend = demoBackend();
    const fixture = await renderShell();

    expect(backend.argsOf('WorkspaceService.GetWorkspaceView')).toContainEqual([1]);
    expect(text(fixture)).toContain('Welcome to Otter');
    expect(text(fixture)).toContain('Payments API');
    expect(text(fixture)).toContain('Version 1.0.0');
  });

  it('offers to create the first workspace', async () => {
    const backend = emptyBackend().on('WorkspaceService.CreateWorkspace', 7);
    const fixture = await renderShell();
    expect(text(fixture)).toContain('No workspace yet');

    await click(fixture, 'Create a workspace');
    await type(fixture, find(fixture, 'app-name-form input') as HTMLInputElement, '  Billing ');
    await click(fixture, 'Create');

    expect(backend.argsOf('WorkspaceService.CreateWorkspace')).toEqual([['Billing']]);
  });

  it('shows the error code Go returns, translated', async () => {
    emptyBackend().on('WorkspaceService.CreateWorkspace', () => {
      throw backendFailure('NAME_REQUIRED');
    });
    const fixture = await renderShell('fr');

    await click(fixture, 'Créer un espace de travail');
    await type(fixture, find(fixture, 'app-name-form input') as HTMLInputElement, 'x');
    await click(fixture, 'Créer');

    expect(find(fixture, '[role="alert"]').textContent).toContain('Saisissez un nom.');
    await click(fixture, '', 'button[aria-label="Fermer"].icon-btn');
    expect(buttons(fixture, '[role="alert"]')).toHaveLength(0);
  });

  it('opens the environment editor from the sidebar', async () => {
    demoBackend();
    const fixture = await renderShell();

    find(fixture, 'button[aria-label="Manage environments"]').click();
    await settle(fixture);

    expect(text(fixture)).toContain('Environments');
    expect((find(fixture, 'td input.mono') as HTMLInputElement).value).toBe('baseUrl');
    await click(fixture, 'Close', '.head button');
    expect(text(fixture)).toContain('Welcome to Otter');
  });

  it('draws its own window buttons outside macOS', async () => {
    demoBackend();
    const fixture = await renderShell();
    const labels = buttons(fixture, '.window-buttons > .window-btn').map((b) =>
      b.getAttribute('aria-label'),
    );

    expect(labels).toEqual(['Minimise', 'Maximise', 'Close']);
  });
});
