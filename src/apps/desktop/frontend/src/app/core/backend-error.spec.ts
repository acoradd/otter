import { backendFailure } from '../../testing/backend';
import { backendError } from './backend-error';

describe('backendError', () => {
  it('reads the code and parameters Go sends', () => {
    expect(backendError(backendFailure('INVALID_INPUT', { field: 'url' }))).toEqual({
      code: 'INVALID_INPUT',
      params: { field: 'url' },
    });
    expect(backendError(backendFailure('NOT_FOUND'))).toEqual({ code: 'NOT_FOUND' });
  });

  it('turns anything else into INTERNAL', () => {
    expect(backendError(new Error('boom'))).toEqual({ code: 'INTERNAL' });
    expect(backendError(null)).toEqual({ code: 'INTERNAL' });
    expect(backendError({ cause: { code: 42 } })).toEqual({ code: 'INTERNAL' });
  });
});
