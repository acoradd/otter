import { TranslocoService } from '@jsverse/transloco';
import { ResolvedVar, Source, SourceKind } from '@bindings/internal/resolve';

/** The level an editor works at: its own rows are "defined here". */
export type Level = 'request' | 'folder';

/**
 * Where a value comes from, as a sentence: "Defined here", "Inherited from
 * api › users", "Environment dev", "Automatic header", "Runtime variable".
 * Go sends the source as data; the words are the frontend's. Depth 0 is the
 * request's folder (inherited) or, in the folder editor, the folder itself.
 */
export function sourceLabel(
  t: TranslocoService,
  source: Source | null | undefined,
  level: Level = 'request',
): string {
  switch (source?.kind) {
    case SourceKind.SourceRequest:
      return t.translate('source.here');
    case SourceKind.SourceFolder:
      return level === 'folder' && !source.depth
        ? t.translate('source.here')
        : t.translate('source.folder', { path: (source.path ?? []).join(' › ') });
    case SourceKind.SourceEnv:
      return t.translate('source.env', { name: source.name });
    case SourceKind.SourceRuntime:
      return t.translate('source.runtime');
    case SourceKind.SourceAuto:
      return t.translate('source.auto');
  }
  return t.translate('source.none');
}

/** Where the request uses a variable: "URL, header Authorization, body". */
export function usedInLabel(t: TranslocoService, usedIn: string[] | null | undefined): string {
  return (usedIn ?? [])
    .map((u) => {
      const [kind, name] = u.split(/:(.*)/s);
      return t.translate(`source.usedIn.${kind}`, { name: name ?? '' });
    })
    .filter((s, i, all) => all.indexOf(s) === i)
    .join(', ');
}

/** What hovering {{name}} tells: value (masked when secret), source, hidden value. */
export function describeVar(
  t: TranslocoService,
  name: string,
  v: ResolvedVar | undefined,
  level: Level = 'request',
): string[] {
  if (!v?.source) {
    return [`{{${name}}}`, t.translate('source.undefined')];
  }
  const mask = (value: string, secret: boolean) =>
    secret ? '••••••' : value === '' ? t.translate('source.empty') : value;
  const lines = [`{{${name}}} = ${mask(v.value, v.secret)}`, sourceLabel(t, v.source, level)];
  const hidden = v.shadowed?.[0];
  if (hidden) {
    lines.push(
      t.translate('source.hides', {
        value: mask(hidden.value, v.secret),
        source: sourceLabel(t, hidden.source),
      }),
    );
  }
  return lines;
}
