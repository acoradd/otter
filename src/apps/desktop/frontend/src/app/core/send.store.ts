import { DestroyRef, Injectable, inject, signal } from '@angular/core';
import { Events } from '@wailsio/runtime';
import { HistoryItem, RequestDraft, ResponseView, SendService } from '@bindings/apps/desktop';

import { BackendError, backendError } from './backend-error';

/** What the response panel of a request shows. */
export interface SendState {
  sending: boolean;
  response: ResponseView | null;
  error: BackendError | null;
  /** The response comes from the history, not the last send. */
  fromHistory: boolean;
  history: HistoryItem[];
}

const EMPTY: SendState = {
  sending: false,
  response: null,
  error: null,
  fromHistory: false,
  history: [],
};

/**
 * Sends requests and keeps, per request, the last response and the history.
 * Sending waits for Go: no optimistic state.
 */
@Injectable({ providedIn: 'root' })
export class SendStore {
  private readonly states = signal<ReadonlyMap<number, SendState>>(new Map());

  constructor() {
    const off = Events.On('history.changed', (e) => {
      if (this.states().has(e.data)) {
        void this.loadHistory(e.data);
      }
    });
    inject(DestroyRef).onDestroy(off);
  }

  state(requestId: number): SendState {
    return this.states().get(requestId) ?? EMPTY;
  }

  /** Sends the draft as the editor holds it. */
  async send(requestId: number, draft: RequestDraft): Promise<void> {
    if (this.state(requestId).sending) {
      return;
    }
    this.set(requestId, { sending: true, error: null });
    try {
      const response = await SendService.SendRequest(requestId, draft);
      this.set(requestId, { sending: false, response, error: null, fromHistory: false });
    } catch (err) {
      this.set(requestId, {
        sending: false,
        response: null,
        error: backendError(err),
        fromHistory: false,
      });
    }
  }

  cancel(requestId: number): void {
    void SendService.CancelRequest(requestId);
  }

  async loadHistory(requestId: number): Promise<void> {
    try {
      const history = (await SendService.GetHistory(requestId)) ?? [];
      this.set(requestId, { history });
    } catch (err) {
      this.set(requestId, { error: backendError(err) });
    }
  }

  /** Shows a past send in the panel. */
  async showHistory(requestId: number, entryId: number): Promise<void> {
    try {
      const response = await SendService.GetHistoryResponse(entryId);
      this.set(requestId, {
        response,
        error: response.error ? { code: response.error } : null,
        fromHistory: true,
      });
    } catch (err) {
      this.set(requestId, { error: backendError(err) });
    }
  }

  /** Asks where to save the body of a response; the path, "" when cancelled. */
  async saveBody(requestId: number, responseId: number): Promise<string> {
    try {
      return await SendService.SaveResponseBody(responseId);
    } catch (err) {
      this.set(requestId, { error: backendError(err) });
      return '';
    }
  }

  private set(requestId: number, change: Partial<SendState>): void {
    this.states.update((m) =>
      new Map(m).set(requestId, { ...(m.get(requestId) ?? EMPTY), ...change }),
    );
  }
}
