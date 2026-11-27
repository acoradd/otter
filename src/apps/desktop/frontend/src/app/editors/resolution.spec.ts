import { TestBed } from '@angular/core/testing';
import { NodeKind, RequestDraft } from '@bindings/apps/desktop';
import { Preview, SourceKind } from '@bindings/internal/resolve';

import { buttons, click, find, render, settle, text } from '../../testing/render';
import { demoBackend, fixtures } from '../../testing/workspace';
import { PREVIEW_DEBOUNCE } from '../core/preview.store';
import { AppSettings } from '../core/settings.store';
import { TabsStore } from '../core/tabs.store';
import { WorkspaceStore } from '../core/workspace.store';
import { Shell } from '../shell/shell';

/** "Create customer" open, its preview arrived (dev: baseUrl, apiKey; requestId undefined). */
async function openRequest(preview: Preview = fixtures.requestPreview(), lang: 'en' | 'fr' = 'en') {
  const backend = demoBackend().on('PreviewService.GetRequestPreview', preview);
  const fixture = await render(Shell, { lang });
  await TestBed.inject(AppSettings).load();
  await TestBed.inject(WorkspaceStore).refresh();
  await TestBed.inject(TabsStore).open('request', 2);
  await settle(fixture);
  await new Promise((r) => setTimeout(r, PREVIEW_DEBOUNCE + 20));
  await settle(fixture);
  return { backend, fixture };
}

type Fixture = Awaited<ReturnType<typeof openRequest>>['fixture'];
const all = <T extends Element>(fixture: Fixture, selector: string) => [
  ...(fixture.nativeElement as HTMLElement).querySelectorAll<T>(selector),
];
const rowText = (r: Element) => r.textContent?.replace(/\s+/g, ' ').trim();
/** The text of each cell, then of each part of a cell, joined by spaces. */
const cells = (r: Element) =>
  [...r.querySelectorAll('td')]
    .map(
      (td) =>
        [...td.querySelectorAll('span, input:not([type="checkbox"])')]
          .map((e) => (e instanceof HTMLInputElement ? e.value : e.textContent?.trim()))
          .filter(Boolean)
          .join(' ') || td.textContent?.trim(),
    )
    .filter(Boolean)
    .join(' ');
const draft = () => TestBed.inject(TabsStore).active()!.draft as RequestDraft;
const input = async (fixture: Fixture, el: HTMLInputElement, value: string) => {
  el.value = value;
  el.dispatchEvent(new Event('input'));
  await settle(fixture);
};

describe('Resolution in the request editor', () => {
  it('previews the draft, not what is saved, shortly after each change', async () => {
    const { backend, fixture } = await openRequest();
    expect(backend.argsOf('PreviewService.GetRequestPreview')).toHaveLength(1);

    await input(
      fixture,
      find(fixture, 'app-var-input.url input') as HTMLInputElement,
      '{{baseUrl}}/a',
    );
    await input(
      fixture,
      find(fixture, 'app-var-input.url input') as HTMLInputElement,
      '{{baseUrl}}/ab',
    );
    await new Promise((r) => setTimeout(r, PREVIEW_DEBOUNCE + 20));

    const calls = backend.argsOf('PreviewService.GetRequestPreview') as [number, RequestDraft][];
    expect(calls).toHaveLength(2); // one for both keystrokes
    expect(calls[1][1].url).toBe('{{baseUrl}}/ab');

    backend.emit('environment.changed', 1);
    await new Promise((r) => setTimeout(r, 20));
    expect(backend.argsOf('PreviewService.GetRequestPreview')).toHaveLength(3);
  });

  it('highlights {{variables}} in the URL, in red when undefined, and tells them on hover', async () => {
    const preview = fixtures.requestPreview();
    preview.url = 'https://dev.payments.test/v1/customers/{{id}}';
    const { fixture } = await openRequest(preview);
    await input(
      fixture,
      find(fixture, 'app-var-input.url input') as HTMLInputElement,
      '{{baseUrl}}/v1/{{requestId}}',
    );

    const vars = all(fixture, 'app-var-input.url .mirror .var');
    expect(vars.map((v) => [v.textContent, v.classList.contains('missing')])).toEqual([
      ['{{baseUrl}}', false],
      ['{{requestId}}', true],
    ]);

    // jsdom has no layout: a character is 7.2 px wide, from x = 0.
    const field = find(fixture, 'app-var-input.url input');
    field.dispatchEvent(new MouseEvent('mousemove', { clientX: 20 }));
    await settle(fixture);
    expect(find(fixture, '[role="tooltip"]').textContent).toContain(
      '{{baseUrl}} = https://dev.payments.test',
    );
    expect(find(fixture, '[role="tooltip"]').textContent).toContain('Environment dev');
    field.dispatchEvent(new MouseEvent('mousemove', { clientX: 7.2 * 20 }));
    await settle(fixture);
    expect(find(fixture, '[role="tooltip"]').textContent).toContain('Not defined anywhere');
  });

  it('merges inherited headers: grey rows, a source on hover, an override in place', async () => {
    const { fixture } = await openRequest();
    const rows = all(fixture, 'app-merged-headers tbody tr');
    expect(rows.map((r) => r.className.split(' ')[0])).toEqual([
      'inherited', // Authorization, from customers
      'inherited', // Content-Type
      'inherited', // User-Agent
      'inherited', // Accept
      'local', // Accept-Encoding, ignored here
      'local', // Idempotency-Key
      'local', // X-Debug, disabled
      'inherited', // Host
      'inherited', // Content-Length
    ]);
    expect(rows[0].querySelector('td.col-name')!.getAttribute('title')).toBe(
      'Inherited from customers',
    );
    expect(rows[1].querySelector('td.col-name')!.getAttribute('title')).toBe('Automatic header');
    expect(rows[4].querySelector('td.col-name')!.getAttribute('title')).toBe(
      'Defined here, replaces: Automatic header',
    );
    expect(cells(rows[7])).toContain('Host dev.payments.test');

    // Unchecking an inherited header ignores it here.
    (rows[3].querySelector('input[type="checkbox"]') as HTMLInputElement).click();
    await settle(fixture);
    expect(draft().headers!.at(-1)).toEqual({
      key: 'Accept',
      value: '',
      ignore: true,
      enabled: true,
    });

    // Typing in an inherited one replaces it here.
    const contentType = all(fixture, 'app-merged-headers tbody tr')[1].querySelector(
      'app-var-input input',
    ) as HTMLInputElement;
    await input(fixture, contentType, 'application/vnd.api+json');
    expect(draft().headers!.at(-1)).toEqual({
      key: 'Content-Type',
      value: 'application/vnd.api+json',
      ignore: false,
      enabled: true,
    });

    // Checking the ignored Accept-Encoding again removes the local row.
    const before = draft().headers!.length;
    const ignored = all(fixture, 'app-merged-headers tbody tr').find((r) =>
      cells(r).startsWith('Accept-Encoding'),
    )!;
    (ignored.querySelector('input[type="checkbox"]') as HTMLInputElement).click();
    await settle(fixture);
    expect(draft().headers!.length).toBe(before - 1);
    expect(draft().headers!.some((h) => h.key === 'Accept-Encoding')).toBe(false);
  });

  it('lists variables: inherited, local, undefined in red with "Define here"', async () => {
    const { fixture } = await openRequest();
    await click(fixture, 'Variables', 'app-request-editor [role="tab"]');
    const rows = all(fixture, 'app-merged-vars tbody tr');
    expect(rows.map((r) => r.className.split(' ')[0])).toEqual([
      'inherited',
      'inherited',
      'inherited',
      'local',
      'missing',
    ]);
    expect(cells(rows[2])).toContain('apiKey ••••••'); // a secret stays masked
    expect(rowText(rows[4])).toContain('requestId Not defined · used in header Idempotency-Key');
    expect(
      find(fixture, 'app-request-editor [role="tab"].active .count.bad').textContent?.trim(),
    ).toBe('1');

    await click(fixture, 'Define here');
    expect(draft().vars!.at(-1)).toEqual({ key: 'requestId', value: '', enabled: true });

    // Typing in an inherited variable defines it here; "back" removes it.
    const resource = all(fixture, 'app-merged-vars tbody tr')[0].querySelector(
      'app-var-input input',
    ) as HTMLInputElement;
    await input(fixture, resource, 'clients');
    expect(draft().vars!.at(-1)).toEqual({ key: 'resource', value: 'clients', enabled: true });
    const back = all(fixture, 'app-merged-vars button[aria-label="Back to the inherited value"]');
    expect(back).toHaveLength(1);
    (back[0] as HTMLButtonElement).click();
    await settle(fixture);
    expect(draft().vars!.some((v) => v.key === 'resource')).toBe(false);
  });

  it('shows runtime variables first, and deletes them through Go', async () => {
    const preview = fixtures.requestPreview();
    preview.vars = [
      {
        key: 'session',
        value: 'abc',
        rawValue: 'abc',
        source: { kind: SourceKind.SourceRuntime },
        shadowed: [],
        secret: false,
        usedIn: [],
      },
      ...(preview.vars ?? []),
    ];
    const { backend, fixture } = await openRequest(preview);
    await click(fixture, 'Variables', 'app-request-editor [role="tab"]');

    const first = all(fixture, 'app-merged-vars tbody tr')[0];
    expect(cells(first)).toContain('session runtime abc');
    (first.querySelector('button') as HTMLButtonElement).click();
    await settle(fixture);
    expect(backend.argsOf('WorkspaceService.DeleteRuntimeVar')).toEqual([[1, 'session']]);
  });

  it('shows warnings in words, braces included', async () => {
    const preview = fixtures.requestPreview();
    preview.warnings = [
      { code: 'VAR_CYCLE', params: { name: 'a' } },
      { code: 'INVALID_URL', params: { url: 'api.test' } },
      { code: 'UNRESOLVED_VAR', params: { name: 'requestId' } },
    ];
    const { fixture } = await openRequest(preview, 'fr');
    expect(all(fixture, '.warnings li').map(rowText)).toEqual([
      '{{a}} fait référence à elle-même, directement ou non (ou trop profondément).',
      'L’URL « api.test » n’est pas une URL http ou https valide.',
    ]);
  });
});

describe('Resolution in the folder editor', () => {
  it('shows what the folder inherits and calls its own rows "here"', async () => {
    const backend = demoBackend();
    const fixture = await render(Shell);
    await TestBed.inject(AppSettings).load();
    await TestBed.inject(WorkspaceStore).refresh();
    await TestBed.inject(TabsStore).open('folder', 1);
    await settle(fixture);
    await new Promise((r) => setTimeout(r, 20));
    await settle(fixture);

    expect(backend.argsOf('PreviewService.GetFolderPreview')[0][0]).toBe(1);
    const rows = all(fixture, 'app-merged-vars tbody tr');
    const own = rows.find(
      (r) => (r.querySelector('input.mono') as HTMLInputElement | null)?.value === 'resource',
    )!;
    expect(own.querySelector('td.col-name')!.getAttribute('title')).toBe('Defined here');
    expect(text(fixture)).toContain('baseUrl'); // from the environment
    expect(
      buttons(fixture, 'app-folder-editor [role="tab"]').map(
        (b) => b.textContent?.trim().split(/\s+/)[0],
      ),
    ).toEqual(['Variables', 'Headers']);
  });
});

it('keeps the tree in step: NodeKind is shared', () => {
  expect(NodeKind.NodeRequest).toBe('request');
});
