import { TranslocoService } from '@jsverse/transloco';

/** The translation key of a Go error code, errors.INTERNAL for an unknown one. */
export function errorKey(transloco: TranslocoService, code: string): string {
  const key = `errors.${code}`;
  return transloco.translate(key) === key ? 'errors.INTERNAL' : key;
}
