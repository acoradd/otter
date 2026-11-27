import { HeaderRow, VarRow } from '@bindings/apps/desktop';
import { ResolvedHeader, ResolvedVar, SourceKind } from '@bindings/internal/resolve';

/**
 * A row of a merged list: a row of the current level (local), what it
 * inherits (inherited), or both when the local row replaces an inherited one.
 */
export interface MergedRow<I> {
  /** Stable identity, so the row keeps its DOM (and focus) when it changes kind. */
  track: string;
  kind: 'local' | 'inherited' | 'readonly' | 'runtime' | 'missing';
  /** Index of the row in the editor's draft, null for a row it does not hold. */
  local: number | null;
  /** The inherited definition this row shows or replaces. */
  inherited: I | null;
}

/**
 * Headers at the current level: each inherited header, replaced by the last
 * enabled local row of that name when there is one, then the other local
 * rows, then the read-only computed headers. Names match without case.
 */
export function mergeHeaders(
  local: HeaderRow[],
  inherited: ResolvedHeader[],
  effective: ResolvedHeader[],
): MergedRow<ResolvedHeader>[] {
  return [
    ...merge(
      local,
      inherited,
      (r) => r.key.trim().toLowerCase(),
      (i) => i.key.toLowerCase(),
      'h:',
    ),
    ...effective
      .filter((h) => h.readOnly)
      .map((h) => ({
        track: 'r:' + h.key.toLowerCase(),
        kind: 'readonly' as const,
        local: null,
        inherited: h,
      })),
  ];
}

/**
 * Variables at the current level: runtime ones first (they win over
 * everything), then each inherited one, replaced by a local row of that
 * name, then the other local rows, then those used but defined nowhere.
 */
export function mergeVars(
  local: VarRow[],
  inherited: ResolvedVar[],
  effective: ResolvedVar[],
): MergedRow<ResolvedVar>[] {
  const runtime = effective
    .filter((v) => v.source?.kind === SourceKind.SourceRuntime)
    .map((v) => ({ track: 'rt:' + v.key, kind: 'runtime' as const, local: null, inherited: v }));
  const missing = effective
    .filter((v) => !v.source)
    .map((v) => ({ track: 'm:' + v.key, kind: 'missing' as const, local: null, inherited: v }));
  return [
    ...runtime,
    ...merge(
      local,
      inherited,
      (r) => r.key.trim(),
      (i) => i.key,
      'v:',
    ),
    ...missing,
  ];
}

function merge<L extends { enabled: boolean }, I>(
  local: L[],
  inherited: I[],
  localName: (r: L) => string,
  inheritedName: (i: I) => string,
  prefix: string,
): MergedRow<I>[] {
  const used = new Set<number>();
  const rows: MergedRow<I>[] = [];
  for (const i of inherited) {
    const name = inheritedName(i);
    let match = -1;
    local.forEach((r, j) => {
      if (r.enabled && !used.has(j) && localName(r) === name) {
        match = j;
      }
    });
    if (match >= 0) {
      used.add(match);
      rows.push({ track: prefix + name, kind: 'local', local: match, inherited: i });
    } else {
      rows.push({ track: prefix + name, kind: 'inherited', local: null, inherited: i });
    }
  }
  local.forEach((_, j) => {
    if (!used.has(j)) {
      rows.push({ track: 'l:' + j, kind: 'local', local: j, inherited: null });
    }
  });
  return rows;
}
