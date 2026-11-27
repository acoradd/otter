import { TestBed } from '@angular/core/testing';

import { backendFailure } from '../../testing/backend';
import { buttons, click, find, render, settle, text, type } from '../../testing/render';
import { demoBackend, fixtures } from '../../testing/workspace';
import { AppSettings } from '../core/settings.store';
import { TabsStore } from '../core/tabs.store';
import { WorkspaceStore } from '../core/workspace.store';
import { Shell } from '../shell/shell';

async function renderShell() {
  const fixture = await render(Shell);
  await TestBed.inject(AppSettings).load();
  await TestBed.inject(WorkspaceStore).refresh();
  await settle(fixture);
  return fixture;
}

type Fixture = Awaited<ReturnType<typeof renderShell>>;
const tabNames = (fixture: Fixture) =>
  buttons(fixture, 'app-tab-bar [role="tab"] .name').map((n) => n.textContent?.trim());
const ctrl = async (fixture: Fixture, key: string) => {
  document.dispatchEvent(new KeyboardEvent('keydown', { key, ctrlKey: true, bubbles: true }));
  await settle(fixture);
};

describe('Tabs', () => {
  it('opens a request from the tree, once', async () => {
    const backend = demoBackend();
    const fixture = await renderShell();
    find(fixture, '[data-row="folder:1"]').click();
    await settle(fixture);

    find(fixture, '[data-row="request:2"]').click();
    await settle(fixture);
    find(fixture, '[data-row="request:2"]').click();
    await settle(fixture);

    expect(backend.argsOf('EditorService.GetRequestView')).toEqual([[2]]);
    expect(tabNames(fixture)).toEqual(['Create customer']);
    expect((find(fixture, 'app-var-input.url input') as HTMLInputElement).value).toContain(
      '/v1/customers',
    );
  });

  it('opens a folder editor from the tree menu', async () => {
    const backend = demoBackend();
    const fixture = await renderShell();
    find(fixture, '[data-row="folder:1"]').dispatchEvent(
      new MouseEvent('contextmenu', { bubbles: true, cancelable: true }),
    );
    await settle(fixture);
    await click(fixture, 'Folder variables and headers', '[role="menuitem"]');

    expect(backend.argsOf('EditorService.GetFolderView')).toEqual([[1]]);
    expect(text(fixture)).toContain('Every request in this folder');
    expect((find(fixture, 'app-merged-vars tr.local input.mono') as HTMLInputElement).value).toBe(
      'resource',
    );

    await click(fixture, 'Headers', 'app-folder-editor [role="tab"]');
    await type(
      fixture,
      find(fixture, 'app-merged-headers tr.local app-var-input input') as HTMLInputElement,
      'Bearer {{token}}',
    );
    await click(fixture, 'Save', 'app-folder-editor button');
    expect(backend.argsOf('EditorService.SaveFolder')).toEqual([
      [
        1,
        {
          ...fixtures.folderView().draft,
          headers: [
            { key: 'Authorization', value: 'Bearer {{token}}', ignore: false, enabled: true },
          ],
        },
      ],
    ]);
  });

  it('closes with Ctrl+W, asking first when there are unsaved changes', async () => {
    const backend = demoBackend();
    const fixture = await renderShell();
    const tabs = TestBed.inject(TabsStore);
    await tabs.open('folder', 1);
    await tabs.open('request', 2);
    await settle(fixture);

    await type(fixture, find(fixture, 'app-var-input.url input') as HTMLInputElement, 'changed');
    await ctrl(fixture, 'w');
    expect(text(fixture)).toContain('Save the changes to “Create customer” before closing?');
    await click(fixture, 'Cancel', '[role="alertdialog"] button');
    expect(tabNames(fixture)).toEqual(['customers', 'Create customer']);

    await ctrl(fixture, 'w');
    await click(fixture, 'Save', '[role="alertdialog"] button');
    expect(backend.called('EditorService.SaveRequest')).toBe(true);
    expect(tabNames(fixture)).toEqual(['customers']);

    // A clean tab closes at once; the welcome comes back.
    await ctrl(fixture, 'w');
    expect(tabNames(fixture)).toEqual([]);
    expect(text(fixture)).toContain('Welcome to Otter');
  });

  it('discards unsaved changes when asked to', async () => {
    const backend = demoBackend();
    const fixture = await renderShell();
    await TestBed.inject(TabsStore).open('request', 2);
    await settle(fixture);
    await type(fixture, find(fixture, 'app-var-input.url input') as HTMLInputElement, 'changed');

    await click(fixture, '', 'app-tab-bar .close');
    await click(fixture, 'Don’t save', '[role="alertdialog"] button');

    expect(backend.called('EditorService.SaveRequest')).toBe(false);
    expect(tabNames(fixture)).toEqual([]);
  });

  it('follows Go: refreshes a clean tab, keeps an edited one, closes a deleted one', async () => {
    const backend = demoBackend();
    const fixture = await renderShell();
    const tabs = TestBed.inject(TabsStore);
    await tabs.open('request', 2);
    await tabs.open('folder', 1);
    await settle(fixture);

    backend.on('EditorService.GetRequestView', {
      ...fixtures.requestView(),
      name: 'Create a customer',
    });
    backend.emit('request.changed', 2);
    await settle(fixture);
    expect(tabNames(fixture)).toEqual(['Create a customer', 'customers']);

    backend.on('EditorService.GetRequestView', () => {
      throw backendFailure('NOT_FOUND');
    });
    backend.emit('tree.changed', { workspaceId: 1, folderIds: [1] });
    await settle(fixture);
    expect(tabNames(fixture)).toEqual(['customers']);
  });

  it('keeps the tabs of each workspace apart', async () => {
    const backend = demoBackend();
    const fixture = await renderShell();
    await TestBed.inject(TabsStore).open('request', 2);
    await settle(fixture);

    backend.emit('settings.changed', { ...fixtures.settings(), lastWorkspaceId: 2 });
    await settle(fixture);
    expect(tabNames(fixture)).toEqual([]);
    backend.emit('settings.changed', fixtures.settings());
    await settle(fixture);
    expect(tabNames(fixture)).toEqual(['Create customer']);
  });
});
