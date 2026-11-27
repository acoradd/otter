import { TestBed } from '@angular/core/testing';
import { RequestDraft } from '@bindings/apps/desktop';

import { backendFailure } from '../../testing/backend';
import { buttons, choose, click, find, render, settle, text, type } from '../../testing/render';
import { demoBackend, fixtures } from '../../testing/workspace';
import { AppSettings } from '../core/settings.store';
import { TabsStore } from '../core/tabs.store';
import { WorkspaceStore } from '../core/workspace.store';
import { Shell } from '../shell/shell';

/** The main window with "Create customer" open in the editor. */
async function openRequest(lang: 'en' | 'fr' = 'en') {
  const fixture = await render(Shell, { lang });
  await TestBed.inject(AppSettings).load();
  await TestBed.inject(WorkspaceStore).refresh();
  await TestBed.inject(TabsStore).open('request', 2);
  await settle(fixture);
  return fixture;
}

type Fixture = Awaited<ReturnType<typeof openRequest>>;

const input = (fixture: Fixture, selector: string) => find(fixture, selector) as HTMLInputElement;
const saved = (): RequestDraft => fixtures.requestView().draft;
const tab = (fixture: Fixture, name: string) => click(fixture, name, '[role="tab"]');

describe('RequestEditor', () => {
  it('shows the request: path, method, URL and headers', async () => {
    demoBackend();
    const fixture = await openRequest();

    expect(find(fixture, '.crumbs').textContent).toContain('customers');
    expect((find(fixture, 'app-request-editor select') as HTMLSelectElement).value).toBe('POST');
    expect(input(fixture, 'input.url').value).toBe('{{baseUrl}}/v1/customers?expand=cards');
    expect(
      buttons(fixture, 'app-tab-bar [role="tab"] .name').map((t) => t.textContent?.trim()),
    ).toEqual(['Create customer']);
    expect(find(fixture, 'app-tab-bar .method').textContent?.trim()).toBe('POST');
    // Headers: one enabled set, one disabled, one ignored (struck through, no value field).
    expect(find(fixture, 'app-request-editor [role="tab"].active').textContent).toContain('2');
    expect(text(fixture)).toContain('Ignored');
  });

  it('marks unsaved changes, and saves the draft with Ctrl+S', async () => {
    const backend = demoBackend();
    const fixture = await openRequest();
    const save = () =>
      buttons(fixture, 'app-request-editor button').find(
        (b) => b.textContent?.trim() === 'Save',
      ) as HTMLButtonElement;
    expect(save().disabled).toBe(true);

    await type(fixture, input(fixture, 'input.url'), '{{baseUrl}}/v2/customers');
    await choose(fixture, find(fixture, 'app-request-editor select') as HTMLSelectElement, 'PUT');
    expect(text(fixture)).toContain('Unsaved changes');
    expect(find(fixture, 'app-tab-bar .close').classList).toContain('dirty');

    document.dispatchEvent(
      new KeyboardEvent('keydown', { key: 's', ctrlKey: true, bubbles: true }),
    );
    await settle(fixture);

    expect(backend.argsOf('EditorService.SaveRequest')).toEqual([
      [2, { ...saved(), method: 'PUT', url: '{{baseUrl}}/v2/customers' }],
    ]);
  });

  it('edits headers and variables in their tables', async () => {
    const backend = demoBackend();
    const fixture = await openRequest();

    await click(fixture, 'Add header');
    const keys = () => [
      ...(fixture.nativeElement as HTMLElement).querySelectorAll<HTMLInputElement>(
        'app-kv-table td input.mono[aria-label="Header"]',
      ),
    ];
    await type(fixture, keys()[3], 'X-Tenant');
    buttons(fixture, 'app-kv-table button[aria-label="Remove"]')[1].click();
    await settle(fixture);
    await tab(fixture, 'Variables');
    await click(fixture, 'Add variable');
    const varKeys = [
      ...(fixture.nativeElement as HTMLElement).querySelectorAll<HTMLInputElement>(
        'app-kv-table td input.mono[aria-label="Variable"]',
      ),
    ];
    await type(fixture, varKeys[1], 'page');
    await click(fixture, 'Save', 'app-request-editor button');

    const [[, draft]] = backend.argsOf('EditorService.SaveRequest') as [[number, RequestDraft]];
    expect(draft.headers!.map((h) => h.key)).toEqual([
      'Idempotency-Key',
      'Accept-Encoding',
      'X-Tenant',
    ]);
    expect(draft.vars!.map((v) => v.key)).toEqual(['locale', 'page']);
  });

  it('edits the body by type, keeping file fields for multipart', async () => {
    const backend = demoBackend().on('EditorService.ChooseFile', '/home/ada/avatar.png');
    const fixture = await openRequest();
    await tab(fixture, 'Body');

    const area = find(fixture, 'app-code-editor textarea') as HTMLTextAreaElement;
    expect(area.value).toContain('Ada Lovelace');
    await type(fixture, area, '{"name":"Grace"}');

    await click(fixture, 'Multipart', '[role="radio"]');
    await click(fixture, 'Add field');
    const last = <T extends HTMLElement>(selector: string) =>
      [...(fixture.nativeElement as HTMLElement).querySelectorAll<T>(selector)].at(-1)!;
    await choose(fixture, last<HTMLSelectElement>('app-body-fields select'), 'file');
    await type(
      fixture,
      last<HTMLInputElement>('app-body-fields input[aria-label="Field"]'),
      'avatar',
    );
    await click(fixture, 'Choose…', 'app-body-fields button');
    expect(backend.argsOf('EditorService.ChooseFile')).toEqual([['']]);
    await click(fixture, 'Save', 'app-request-editor button');

    const [[, draft]] = backend.argsOf('EditorService.SaveRequest') as [[number, RequestDraft]];
    expect(draft.bodyType).toBe('multipart');
    expect(draft.body).toBe('{"name":"Grace"}'); // kept for when JSON comes back
    expect(draft.bodyFields!.at(-1)).toEqual({
      key: 'avatar',
      file: true,
      value: '/home/ada/avatar.png',
      contentType: '',
      enabled: true,
    });
  });

  it('picks the file of a file body', async () => {
    const backend = demoBackend().on('EditorService.ChooseFile', '/data/export.bin');
    const fixture = await openRequest();
    await tab(fixture, 'Body');
    await click(fixture, 'File', '[role="radio"]');
    await click(fixture, 'Choose…');

    expect(input(fixture, '.file-row input').value).toBe('/data/export.bin');
    expect(backend.called('EditorService.SaveRequest')).toBe(false);
  });

  it('edits the script and the settings', async () => {
    const backend = demoBackend();
    const fixture = await openRequest();
    await tab(fixture, 'Script');
    await type(
      fixture,
      find(fixture, 'app-code-editor textarea') as HTMLTextAreaElement,
      'console.log(res.status)',
    );
    await tab(fixture, 'Settings');
    const checks = [
      ...(fixture.nativeElement as HTMLElement).querySelectorAll<HTMLInputElement>(
        '.settings input[type="checkbox"]',
      ),
    ];
    checks[0].click(); // encode URL off
    await settle(fixture);
    const numbers = [
      ...(fixture.nativeElement as HTMLElement).querySelectorAll<HTMLInputElement>(
        '.settings input[type="number"]',
      ),
    ];
    await type(fixture, numbers[1], '5000');
    await click(fixture, 'Save', 'app-request-editor button');

    const [[, draft]] = backend.argsOf('EditorService.SaveRequest') as [[number, RequestDraft]];
    expect(draft).toEqual({
      ...saved(),
      postScript: 'console.log(res.status)',
      encodeUrl: false,
      timeoutMs: 5000,
    });
  });

  it('shows the code Go refuses a save with, and reverts', async () => {
    demoBackend().on('EditorService.SaveRequest', () => {
      throw backendFailure('INVALID_METHOD', { method: 'GET X' });
    });
    const fixture = await openRequest('fr');
    await type(fixture, input(fixture, 'input.url'), 'x');
    await click(fixture, 'Enregistrer', 'app-request-editor button');

    expect(find(fixture, 'app-request-editor [role="alert"]').textContent).toContain(
      '« GET X » n’est pas une méthode HTTP valide.',
    );
    await click(fixture, 'Annuler les modifications');
    expect(input(fixture, 'input.url').value).toBe(saved().url);
  });
});
