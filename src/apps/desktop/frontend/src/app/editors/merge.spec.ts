import { HeaderRow, VarRow } from '@bindings/apps/desktop';
import { ResolvedHeader, ResolvedVar, SourceKind } from '@bindings/internal/resolve';

import { mergeHeaders, mergeVars } from './merge';

const header = (key: string, extra: Partial<ResolvedHeader> = {}): ResolvedHeader => ({
  key,
  rawValue: 'v',
  value: 'v',
  source: { kind: SourceKind.SourceFolder, depth: 1 },
  shadowed: [],
  ignored: false,
  readOnly: false,
  computed: false,
  secret: false,
  ...extra,
});
const row = (key: string, extra: Partial<HeaderRow> = {}): HeaderRow => ({
  key,
  value: 'x',
  ignore: false,
  enabled: true,
  ...extra,
});
const variable = (key: string, kind: SourceKind | null): ResolvedVar => ({
  key,
  value: 'v',
  rawValue: 'v',
  source: kind ? { kind } : null,
  shadowed: [],
  secret: false,
  usedIn: [],
});
const vrow = (key: string, enabled = true): VarRow => ({ key, value: 'x', enabled });

const shape = (rows: { track: string; kind: string; local: number | null }[]) =>
  rows.map((r) => `${r.track} ${r.kind}${r.local === null ? '' : ' #' + r.local}`);

describe('mergeHeaders', () => {
  it('shows inherited rows, replaced in place by local rows of the same name', () => {
    const rows = mergeHeaders(
      [row('X-Local'), row('accept'), row(' User-Agent ', { ignore: true })],
      [header('Authorization'), header('Accept'), header('User-Agent')],
      [header('Host', { readOnly: true })],
    );
    expect(shape(rows)).toEqual([
      'h:authorization inherited',
      'h:accept local #1',
      'h:user-agent local #2',
      'l:0 local #0',
      'r:host readonly',
    ]);
  });

  it('lets a disabled local row show the inherited one again', () => {
    const rows = mergeHeaders([row('Accept', { enabled: false })], [header('Accept')], []);
    expect(shape(rows)).toEqual(['h:accept inherited', 'l:0 local #0']);
  });

  it('takes the last enabled row of a name, the others stay local', () => {
    const rows = mergeHeaders([row('Accept'), row('Accept')], [header('Accept')], []);
    expect(shape(rows)).toEqual(['h:accept local #1', 'l:0 local #0']);
  });
});

describe('mergeVars', () => {
  it('puts runtime first and undefined ones last; names match with case', () => {
    const rows = mergeVars(
      [vrow('token'), vrow('Base')],
      [variable('base', SourceKind.SourceEnv), variable('token', SourceKind.SourceFolder)],
      [
        variable('session', SourceKind.SourceRuntime),
        variable('id', null),
        variable('token', SourceKind.SourceRequest),
      ],
    );
    expect(shape(rows)).toEqual([
      'rt:session runtime',
      'v:base inherited',
      'v:token local #0',
      'l:1 local #1',
      'm:id missing',
    ]);
  });
});
