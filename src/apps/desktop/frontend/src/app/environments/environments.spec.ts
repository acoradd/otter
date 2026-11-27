import { TestBed } from '@angular/core/testing';

import { backendFailure } from '../../testing/backend';
import { buttons, click, find, render, settle, text, type } from '../../testing/render';
import { demoBackend, fixtures } from '../../testing/workspace';
import { AppSettings } from '../core/settings.store';
import { WorkspaceStore } from '../core/workspace.store';
import { Environments } from './environments';

async function renderEditor(lang: 'en' | 'fr' = 'en') {
  const fixture = await render(Environments, { lang });
  await TestBed.inject(AppSettings).load();
  await TestBed.inject(WorkspaceStore).refresh();
  await settle(fixture);
  return fixture;
}

const inputs = (fixture: Parameters<typeof find>[0], selector: string) => [
  ...(fixture.nativeElement as HTMLElement).querySelectorAll<HTMLInputElement>(selector),
];

describe('Environments', () => {
  it('shows the active environment and its variables, secrets masked', async () => {
    const backend = demoBackend();
    const fixture = await renderEditor();

    expect(backend.argsOf('WorkspaceService.GetEnvironmentView')).toEqual([[1]]);
    expect(find(fixture, '.list-item.active').textContent).toContain('dev');
    expect(find(fixture, '.list-item.active').textContent).toContain('Active');
    expect(inputs(fixture, 'td input.mono[aria-label="Name"]').map((i) => i.value)).toEqual([
      'baseUrl',
      'apiKey',
      'tenant',
    ]);
    const values = inputs(fixture, 'td input.mono[aria-label="Value"]');
    expect(values.map((i) => i.type)).toEqual(['text', 'password', 'text']);

    find(fixture, 'button[aria-label="Show value"]').click();
    await settle(fixture);
    expect(inputs(fixture, 'td input.mono[aria-label="Value"]')[1].type).toBe('text');
  });

  it('saves the edited variables in order, and only when changed', async () => {
    const backend = demoBackend();
    const fixture = await renderEditor();
    const save = () =>
      buttons(fixture).find((b) => b.textContent?.trim() === 'Save') as HTMLButtonElement;
    expect(save().disabled).toBe(true);

    await type(
      fixture,
      inputs(fixture, 'td input.mono[aria-label="Value"]')[0],
      'https://dev2.payments.test',
    );
    buttons(fixture, 'button[aria-label="Remove variable"]')[2].click();
    await click(fixture, 'Add variable');
    await type(fixture, inputs(fixture, 'td input.mono[aria-label="Name"]')[2], 'region');
    expect(text(fixture)).toContain('Unsaved changes');
    await click(fixture, 'Save');

    expect(backend.argsOf('WorkspaceService.SaveEnvironmentVars')).toEqual([
      [
        1,
        [
          { key: 'baseUrl', value: 'https://dev2.payments.test', secret: false, enabled: true },
          { key: 'apiKey', value: 'dev-4f9a2c', secret: true, enabled: true },
          { key: 'region', value: '', secret: false, enabled: true },
        ],
      ],
    ]);
  });

  it('shows a refused variable name, and reverts', async () => {
    demoBackend().on('WorkspaceService.SaveEnvironmentVars', () => {
      throw backendFailure('INVALID_VAR_KEY', { key: 'base url' });
    });
    const fixture = await renderEditor('fr');

    await type(fixture, inputs(fixture, 'td input.mono[aria-label="Nom"]')[0], 'base url');
    await click(fixture, 'Enregistrer');
    expect(find(fixture, '[role="alert"]').textContent).toContain(
      '« base url » n’est pas un nom de variable valide',
    );

    await click(fixture, 'Annuler les modifications');
    expect(inputs(fixture, 'td input.mono[aria-label="Nom"]')[0].value).toBe('baseUrl');
  });

  it('creates, renames and deletes an environment', async () => {
    const backend = demoBackend().on('WorkspaceService.CreateEnvironment', 4);
    const fixture = await renderEditor();
    const input = () => find(fixture, 'app-name-form input') as HTMLInputElement;

    await click(fixture, 'New environment');
    await type(fixture, input(), 'qa');
    await click(fixture, 'Create');

    await click(fixture, 'staging', '.list-item');
    expect(backend.argsOf('WorkspaceService.GetEnvironmentView').at(-1)).toEqual([2]);
    await click(fixture, 'Rename', '.editor-head button');
    await type(fixture, input(), 'preprod');
    await click(fixture, 'Rename', 'app-name-form button');
    await click(fixture, 'Delete', '.editor-head button');
    await click(fixture, 'Delete', '.confirm button');

    expect(backend.argsOf('WorkspaceService.CreateEnvironment')).toEqual([[1, 'qa']]);
    expect(backend.argsOf('WorkspaceService.RenameEnvironment')).toEqual([[2, 'preprod']]);
    expect(backend.argsOf('WorkspaceService.DeleteEnvironment')).toEqual([[2]]);
  });

  it('invites to create one when the workspace has none', async () => {
    demoBackend().on('WorkspaceService.GetWorkspaceView', {
      ...fixtures.workspaceView(),
      activeEnvironmentId: 0,
      environments: [],
    });
    const fixture = await renderEditor();

    expect(text(fixture)).toContain('No environment yet.');
  });
});
