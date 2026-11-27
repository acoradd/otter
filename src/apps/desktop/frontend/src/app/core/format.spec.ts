import { formatBytes, formatDuration, prettyJson } from './format';

describe('format', () => {
  it('formats sizes', () => {
    expect(formatBytes(0, 'en')).toBe('0 B');
    expect(formatBytes(512, 'en')).toBe('512 B');
    expect(formatBytes(1536, 'en')).toBe('1.5 KB');
    expect(formatBytes(1536, 'fr')).toBe('1,5 KB');
    expect(formatBytes(5 * 1024 * 1024, 'en')).toBe('5 MB');
  });

  it('formats durations', () => {
    expect(formatDuration(42, 'en')).toBe('42 ms');
    expect(formatDuration(1250, 'en')).toBe('1.25 s');
    expect(formatDuration(125_000, 'en')).toBe('2 min 5 s');
  });

  it('indents JSON and leaves the rest', () => {
    expect(prettyJson('{"a":[1]}')).toBe('{\n  "a": [\n    1\n  ]\n}');
    expect(prettyJson('<a/>')).toBe('<a/>');
  });
});
