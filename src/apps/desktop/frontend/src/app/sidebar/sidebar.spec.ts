import { TestBed } from '@angular/core/testing';

import { choose, click, find, render, settle, text, type } from '../../testing/render';
import { demoBackend } from '../../testing/workspace';
import { AppSettings } from '../core/settings.store';
import { WorkspaceStore } from '../core/workspace.store';
import { Sidebar } from './sidebar';

async function renderSidebar() {
  const fixture = await render(Sidebar);
  await TestBed.inject(AppSettings).load();
  await TestBed.inject(WorkspaceStore).refresh();
  await settle(fixture);
  return fixture;
}

describe('Sidebar', () => {
  it('lists the workspaces and the environments of the open one', async () => {
    demoBackend();
    const fixture = await renderSidebar();
    const workspace = find(fixture, '#workspace') as HTMLSelectElement;
    const environment = find(fixture, '#environment') as HTMLSelectElement;

    expect([...workspace.options].map((o) => o.text)).toEqual(['Identity', 'Payments API']);
    expect(workspace.selectedOptions[0].text).toBe('Payments API');
    expect([...environment.options].map((o) => o.text)).toEqual([
      'No environment',
      'dev',
      'staging',
      'production',
    ]);
    expect(environment.selectedOptions[0].text).toBe('dev');
  });

  it('opens another workspace and changes the active environment', async () => {
    const backend = demoBackend();
    const fixture = await renderSidebar();

    await choose(fixture, find(fixture, '#workspace') as HTMLSelectElement, '2');
    await choose(fixture, find(fixture, '#environment') as HTMLSelectElement, '0');

    expect(backend.argsOf('WorkspaceService.OpenWorkspace')).toEqual([[2]]);
    expect(backend.argsOf('WorkspaceService.SetActiveEnvironment')).toEqual([[1, 0]]);
  });

  it('creates, renames and deletes a workspace', async () => {
    const backend = demoBackend().on('WorkspaceService.CreateWorkspace', 3);
    const fixture = await renderSidebar();
    const input = () => find(fixture, 'app-name-form input') as HTMLInputElement;

    find(fixture, 'button[aria-label="New workspace"]').click();
    await settle(fixture);
    await type(fixture, input(), 'Orders');
    await click(fixture, 'Create');

    find(fixture, 'button[aria-label="Rename workspace"]').click();
    await settle(fixture);
    expect(input().value).toBe('Payments API');
    await type(fixture, input(), 'Payments');
    await click(fixture, 'Rename');

    find(fixture, 'button[aria-label="Delete workspace"]').click();
    await settle(fixture);
    expect(text(fixture)).toContain('Delete “Payments API” and everything in it?');
    await click(fixture, 'Delete');

    expect(backend.argsOf('WorkspaceService.CreateWorkspace')).toEqual([['Orders']]);
    expect(backend.argsOf('WorkspaceService.RenameWorkspace')).toEqual([[1, 'Payments']]);
    expect(backend.argsOf('WorkspaceService.DeleteWorkspace')).toEqual([[1]]);
  });

  it('refetches the workspace when Go says its environments changed', async () => {
    const backend = demoBackend();
    const fixture = await renderSidebar();
    const before = backend.argsOf('WorkspaceService.GetWorkspaceView').length;

    backend.emit('environment.changed', 1);
    await settle(fixture);
    backend.emit('environment.changed', 2); // another workspace: ignored
    await settle(fixture);

    expect(backend.argsOf('WorkspaceService.GetWorkspaceView').length).toBe(before + 1);
  });
});
