/** 0 B, 512 B, 1.2 KB, 3.4 MB: sizes in the UI language. */
export function formatBytes(bytes: number, lang: string): string {
  const units = ['B', 'KB', 'MB', 'GB'];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  const digits = unit === 0 || value >= 100 ? 0 : 1;
  return `${new Intl.NumberFormat(lang, { maximumFractionDigits: digits }).format(value)} ${units[unit]}`;
}

/** 42 ms, 1.25 s, 2 min 5 s. */
export function formatDuration(ms: number, lang: string): string {
  const n = (v: number, d = 0) =>
    new Intl.NumberFormat(lang, { maximumFractionDigits: d }).format(v);
  if (ms < 1000) {
    return `${n(ms)} ms`;
  }
  if (ms < 60_000) {
    return `${n(ms / 1000, 2)} s`;
  }
  return `${n(Math.floor(ms / 60_000))} min ${n(Math.round((ms % 60_000) / 1000))} s`;
}

/** The date and time of a send, in the UI language. */
export function formatDateTime(epochMs: number, lang: string): string {
  return new Intl.DateTimeFormat(lang, { dateStyle: 'short', timeStyle: 'medium' }).format(
    new Date(epochMs),
  );
}

/** A JSON body indented for reading; anything else as it is. */
export function prettyJson(text: string): string {
  try {
    return JSON.stringify(JSON.parse(text), null, 2);
  } catch {
    return text;
  }
}
