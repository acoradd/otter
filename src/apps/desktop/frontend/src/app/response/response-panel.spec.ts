import { TestBed } from '@angular/core/testing';
import { RequestDraft, ResponseView } from '@bindings/apps/desktop';

import { backendFailure } from '../../testing/backend';
import { buttons, click, find, render, settle, text } from '../../testing/render';
import { demoBackend, fixtures } from '../../testing/workspace';
import { AppSettings } from '../core/settings.store';
import { TabsStore } from '../core/tabs.store';
import { WorkspaceStore } from '../core/workspace.store';
import { Shell } from '../shell/shell';

async function openRequest(lang: 'en' | 'fr' = 'en') {
  const fixture = await render(Shell, { lang });
  await TestBed.inject(AppSettings).load();
  await TestBed.inject(WorkspaceStore).refresh();
  await TestBed.inject(TabsStore).open('request', 2);
  await settle(fixture);
  return fixture;
}

type Fixture = Awaited<ReturnType<typeof openRequest>>;
const panel = (fixture: Fixture) => find(fixture, 'app-response-panel');
const ctrlEnter = async (fixture: Fixture) => {
  document.dispatchEvent(
    new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, bubbles: true }),
  );
  await settle(fixture);
};

describe('Sending', () => {
  it('sends the draft as it is, and shows the response', async () => {
    const backend = demoBackend().on('SendService.SendRequest', fixtures.responseView());
    const fixture = await openRequest();
    expect(panel(fixture).textContent).toContain('Send the request to see its response');

    await click(fixture, 'Send', 'app-request-editor .send');

    const [[id, draft]] = backend.argsOf('SendService.SendRequest') as [[number, RequestDraft]];
    expect(id).toBe(2);
    expect(draft.url).toBe(fixtures.requestView().draft.url);
    const summary = find(fixture, 'app-response-panel .summary').textContent!.replace(/\s+/g, ' ');
    expect(summary).toContain('201 Created');
    expect(summary).toContain('42 ms');
    expect(summary).toContain('75 B');
    // JSON is shown indented; Raw shows it as it came.
    const body = () =>
      (find(fixture, 'app-response-panel app-code-editor textarea') as HTMLTextAreaElement).value;
    expect(body()).toContain('\n  "name": "Ada Lovelace"');
    await click(fixture, 'Raw', 'app-response-panel .seg button');
    expect(body()).toBe(fixtures.responseView().body);
  });

  it('sends with Ctrl+Enter, and cancels while it runs', async () => {
    let answer!: (v: ResponseView) => void;
    const backend = demoBackend().on(
      'SendService.SendRequest',
      () => new Promise((resolve) => (answer = resolve)),
    );
    const fixture = await openRequest();

    await ctrlEnter(fixture);
    expect(backend.called('SendService.SendRequest')).toBe(true);
    expect(panel(fixture).textContent).toContain('Sending…');
    await click(fixture, 'Cancel', 'app-request-editor .send');
    expect(backend.argsOf('SendService.CancelRequest')).toEqual([[2]]);

    await ctrlEnter(fixture); // already sending: ignored
    expect(backend.argsOf('SendService.SendRequest')).toHaveLength(1);
    answer(fixtures.responseView());
    await settle(fixture);
    expect(panel(fixture).textContent).toContain('201 Created');
  });

  it('shows why a send failed, in words', async () => {
    demoBackend().on('SendService.SendRequest', () => {
      throw backendFailure('HOST_NOT_FOUND', { host: 'dev.payments.test' });
    });
    const fixture = await openRequest('fr');
    await click(fixture, 'Envoyer', 'app-request-editor .send');
    expect(find(fixture, 'app-response-panel [role="alert"]').textContent).toContain(
      'L’hôte dev.payments.test est introuvable (DNS).',
    );
  });

  it('lists headers, what was sent (secrets masked) and redirects', async () => {
    const view = fixtures.responseView();
    view.redirects = [
      { status: 302, url: 'https://dev.payments.test/old', location: '/v1/customers' },
    ];
    view.sentHeaders = [...(view.sentHeaders ?? []), { key: 'Authorization', value: '••••••' }];
    demoBackend().on('SendService.SendRequest', view);
    const fixture = await openRequest();
    await click(fixture, 'Send', 'app-request-editor .send');

    await click(fixture, 'Headers', 'app-response-panel [role="tab"]');
    expect(text(fixture)).toContain('X-Request-Id');
    await click(fixture, 'Request sent', 'app-response-panel [role="tab"]');
    expect(text(fixture)).toContain('POST https://dev.payments.test/v1/customers?expand=cards');
    expect(text(fixture)).not.toContain('dev-4f9a2c');
    await click(fixture, 'Redirects', 'app-response-panel [role="tab"]');
    expect(find(fixture, '.redirects').textContent).toContain(
      '302 https://dev.payments.test/old → /v1/customers',
    );
  });

  it('offers to save a binary body to a file', async () => {
    const view: ResponseView = {
      ...fixtures.responseView(),
      bodyKind: 'binary',
      body: '',
      contentType: 'image/png',
      size: 2048,
    };
    const backend = demoBackend()
      .on('SendService.SendRequest', view)
      .on('SendService.SaveResponseBody', '/home/ada/avatar.png');
    const fixture = await openRequest();
    await click(fixture, 'Send', 'app-request-editor .send');

    expect(panel(fixture).textContent).toContain('The body is binary (2 KB, image/png).');
    await click(fixture, 'Save to a file', 'app-response-panel button');
    expect(backend.argsOf('SendService.SaveResponseBody')).toEqual([[view.id]]);
    expect(panel(fixture).textContent).toContain('Saved to /home/ada/avatar.png');
  });

  it('keeps the history: refreshed after a send, a past send shown on click', async () => {
    const backend = demoBackend()
      .on('SendService.GetHistory', fixtures.history())
      .on('SendService.GetHistoryResponse', fixtures.responseView());
    const fixture = await openRequest();
    expect(backend.argsOf('SendService.GetHistory')).toContainEqual([2]);

    await click(fixture, 'History', 'app-response-panel [role="tab"]');
    const rows = buttons(fixture, 'app-response-panel .history tr');
    expect(rows).toHaveLength(1);
    expect(rows[0].textContent).toContain('201');
    rows[0].click();
    await settle(fixture);
    expect(backend.argsOf('SendService.GetHistoryResponse')).toEqual([[1]]);
    expect(panel(fixture).textContent).toContain('From the history');

    const before = backend.argsOf('SendService.GetHistory').length;
    backend.emit('history.changed', 2);
    await settle(fixture);
    expect(backend.argsOf('SendService.GetHistory').length).toBe(before + 1);
  });
});
