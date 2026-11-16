/** Error sent by Go as the "cause" of a rejected call: a stable code and its parameters. */
export interface BackendError {
  code: string;
  params?: Record<string, unknown>;
}

/** Reads the cause of a rejected binding call; anything else is INTERNAL. */
export function backendError(err: unknown): BackendError {
  const cause = (err as { cause?: { code?: unknown; params?: unknown } } | null)?.cause;
  if (typeof cause?.code !== 'string') {
    return { code: 'INTERNAL' };
  }
  const params = cause.params;
  return params !== null && typeof params === 'object'
    ? { code: cause.code, params: params as Record<string, unknown> }
    : { code: cause.code };
}
