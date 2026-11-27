import { TestBed } from '@angular/core/testing';
import { NodeKind } from '@bindings/apps/desktop';

import { backendFailure } from '../../testing/backend';
import { buttons, click, drag, find, render, settle, text, type } from '../../testing/render';
import { demoBackend, fixtures } from '../../testing/workspace';
import { AppSettings } from '../core/settings.store';
import { WorkspaceStore } from '../core/workspace.store';
import { Tree } from './tree';

async function renderTree(lang: 'en' | 'fr' = 'en') {
  const fixture = await render(Tree, { lang });
  await TestBed.inject(AppSettings).load();
  await TestBed.inject(WorkspaceStore).refresh();
  await settle(fixture);
  return fixture;
}

type Fixture = Awaited<ReturnType<typeof renderTree>>;

const row = (fixture: Fixture, key: string) => find(fixture, `[data-row="${key}"]`);
const rowNames = (fixture: Fixture) =>
  buttons(fixture, '[role="treeitem"]').map((r) =>
    [r.querySelector('.method')?.textContent, r.querySelector('.name')?.textContent]
      .filter(Boolean)
      .join(' ')
      .trim(),
  );

async function expand(fixture: Fixture, key: string) {
  row(fixture, key).click();
  await settle(fixture);
}

async function contextMenu(fixture: Fixture, el: HTMLElement) {
  el.dispatchEvent(
    new MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: 10, clientY: 10 }),
  );
  await settle(fixture);
}

describe('Tree', () => {
  it('shows the root folders and loads a folder only when it is expanded', async () => {
    const backend = demoBackend();
    const fixture = await renderTree();

    expect(rowNames(fixture)).toEqual(['customers', 'payments', 'webhooks']);
    expect(backend.argsOf('TreeService.GetChildren')).toEqual([[1, 0]]);

    await expand(fixture, 'folder:1');
    expect(backend.argsOf('TreeService.GetChildren')).toEqual([
      [1, 0],
      [1, 1],
    ]);
    expect(rowNames(fixture)).toEqual([
      'customers',
      'cards',
      'GET List customers',
      'POST Create customer',
      'payments',
      'webhooks',
    ]);
    expect(row(fixture, 'request:2').querySelector('.method')?.classList).toContain('method-post');

    // Collapsing then expanding again uses the cache.
    await expand(fixture, 'folder:1');
    await expand(fixture, 'folder:1');
    expect(backend.argsOf('TreeService.GetChildren')).toHaveLength(2);
    expect(row(fixture, 'folder:1').getAttribute('aria-expanded')).toBe('true');
  });

  it('refetches only the loaded folders Go says changed', async () => {
    const backend = demoBackend();
    const fixture = await renderTree();
    await expand(fixture, 'folder:1');

    backend.emit('tree.changed', { workspaceId: 1, folderIds: [1, 2] });
    await settle(fixture);
    backend.emit('tree.changed', { workspaceId: 2, folderIds: [0] });
    await settle(fixture);

    expect(backend.argsOf('TreeService.GetChildren')).toEqual([
      [1, 0],
      [1, 1],
      [1, 1],
    ]);
  });

  it('creates a folder and a request from the menu, then names them in place', async () => {
    const backend = demoBackend()
      .on('TreeService.CreateFolder', 9)
      .on('TreeService.CreateRequest', 20);
    const fixture = await renderTree();

    await contextMenu(fixture, row(fixture, 'folder:3'));
    await click(fixture, 'New request', '[role="menuitem"]');
    expect(backend.argsOf('TreeService.CreateRequest')).toEqual([[3, 'New request']]);

    await contextMenu(fixture, find(fixture, 'cdk-virtual-scroll-viewport'));
    expect(buttons(fixture, '[role="menuitem"]').map((b) => b.textContent?.trim())).toEqual([
      'New folder',
    ]);
    await click(fixture, 'New folder', '[role="menuitem"]');
    expect(backend.argsOf('TreeService.CreateFolder')).toEqual([[1, 0, 'New folder']]);
  });

  it('renames in place with F2 and Enter', async () => {
    const backend = demoBackend();
    const fixture = await renderTree();
    row(fixture, 'folder:2').click();
    await settle(fixture);

    find(fixture, 'cdk-virtual-scroll-viewport').dispatchEvent(
      new KeyboardEvent('keydown', { key: 'F2', bubbles: true }),
    );
    await settle(fixture);
    const input = find(fixture, '.rename input') as HTMLInputElement;
    expect(input.value).toBe('payments');
    await type(fixture, input, 'billing');
    input.form!.requestSubmit();
    await settle(fixture);

    expect(backend.argsOf('TreeService.RenameNode')).toEqual([[NodeKind.NodeFolder, 2, 'billing']]);
  });

  it('duplicates with a translated name', async () => {
    const backend = demoBackend();
    const fixture = await renderTree('fr');

    await contextMenu(fixture, row(fixture, 'folder:2'));
    await click(fixture, 'Dupliquer', '[role="menuitem"]');

    expect(backend.argsOf('TreeService.DuplicateNode')).toEqual([
      [NodeKind.NodeFolder, 2, 'payments (copie)'],
    ]);
  });

  it('deletes after a confirmation', async () => {
    const backend = demoBackend();
    const fixture = await renderTree();

    await contextMenu(fixture, row(fixture, 'folder:1'));
    await click(fixture, 'Delete', '[role="menuitem"]');
    expect(text(fixture)).toContain('Delete the folder “customers” and everything in it?');
    await click(fixture, 'Cancel', '[role="alertdialog"] button');
    expect(backend.called('TreeService.DeleteNode')).toBe(false);

    row(fixture, 'folder:1').click();
    await settle(fixture);
    find(fixture, 'cdk-virtual-scroll-viewport').dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Delete', bubbles: true }),
    );
    await settle(fixture);
    await click(fixture, 'Delete', '[role="alertdialog"] button');
    expect(backend.argsOf('TreeService.DeleteNode')).toEqual([[NodeKind.NodeFolder, 1]]);
  });

  it('moves a request into a folder by dragging it', async () => {
    const backend = demoBackend();
    const fixture = await renderTree();
    await expand(fixture, 'folder:1');

    await drag(fixture, row(fixture, 'request:1'), row(fixture, 'folder:2'));

    expect(backend.argsOf('TreeService.MoveNode')).toEqual([[NodeKind.NodeRequest, 1, 2, -1]]);
  });

  it('refuses to drag a folder into itself, and shows the code Go returns', async () => {
    const backend = demoBackend().on('TreeService.MoveNode', () => {
      throw backendFailure('FOLDER_CYCLE');
    });
    const fixture = await renderTree();
    await expand(fixture, 'folder:1');

    await drag(fixture, row(fixture, 'folder:1'), row(fixture, 'folder:4'));
    expect(backend.called('TreeService.MoveNode')).toBe(false);

    await drag(fixture, row(fixture, 'folder:2'), row(fixture, 'folder:3'));
    expect(backend.argsOf('TreeService.MoveNode')).toEqual([[NodeKind.NodeFolder, 2, 3, -1]]);
    expect(TestBed.inject(WorkspaceStore).error()).toEqual({ code: 'FOLDER_CYCLE' });
  });

  it('moves through "Move to", filtered by path', async () => {
    const backend = demoBackend();
    const fixture = await renderTree();
    await expand(fixture, 'folder:1');

    await contextMenu(fixture, row(fixture, 'request:2'));
    await click(fixture, 'Move to…', '[role="menuitem"]');
    expect(backend.argsOf('TreeService.GetFolderChoices')).toEqual([[1]]);
    const options = () => buttons(fixture, '[role="option"]').map((b) => b.textContent?.trim());
    expect(options()).not.toContain('Workspace root'); // requests live in folders
    await type(fixture, find(fixture, '[role="dialog"] input') as HTMLInputElement, 'CARD');
    expect(options()).toEqual(['customers › cards']);
    await click(fixture, 'customers › cards', '[role="option"]');

    expect(backend.argsOf('TreeService.MoveNode')).toEqual([[NodeKind.NodeRequest, 2, 4, -1]]);
  });

  it('invites to create the first folder of an empty workspace', async () => {
    const backend = demoBackend()
      .on('TreeService.GetChildren', [])
      .on('TreeService.CreateFolder', 1);
    const fixture = await renderTree();

    expect(text(fixture)).toContain('No folders yet.');
    await click(fixture, 'New folder', '.first button');
    expect(backend.argsOf('TreeService.CreateFolder')).toEqual([[1, 0, 'New folder']]);
  });

  it('keeps the fixtures in step with the tree the fake Go serves', () => {
    expect([...fixtures.tree().keys()]).toEqual([0, 1, 4]);
  });
});
