/** How a response body is shown. */
export type BodyFormat = 'json' | 'html' | 'xml' | 'plaintext';

/** HTML elements that tell a page from an XML document. */
const HTML_HINT =
  /<(html|head|body|div|span|p|a|br|table|ul|li|script|style|meta|link|title|h[1-6])[\s>/]/i;

/**
 * The format of a body: from its Content-Type when it says, else from the
 * content itself. detected tells which one decided.
 */
export function bodyFormat(
  contentType: string,
  body: string,
): { format: BodyFormat; detected: boolean } {
  const t = contentType.toLowerCase();
  if (t.includes('json')) {
    return { format: 'json', detected: false };
  }
  if (t.includes('html')) {
    return { format: 'html', detected: false };
  }
  if (t.includes('xml')) {
    return { format: 'xml', detected: false };
  }
  const format = sniff(body);
  return { format, detected: format !== 'plaintext' };
}

/** Recognizes JSON, HTML or XML from the start of a text. */
export function sniff(body: string): BodyFormat {
  const s = body.trimStart().slice(0, 1024);
  if (s.startsWith('{') || s.startsWith('[')) {
    try {
      JSON.parse(body);
      return 'json';
    } catch {
      return 'plaintext';
    }
  }
  if (!s.startsWith('<')) {
    return 'plaintext';
  }
  if (/^<!doctype\s+html/i.test(s) || /^<html[\s>]/i.test(s)) {
    return 'html';
  }
  if (/^<\?xml/i.test(s)) {
    return 'xml';
  }
  if (/<\/[\w:.-]+\s*>|\/>/.test(body)) {
    return HTML_HINT.test(s) ? 'html' : 'xml';
  }
  return 'plaintext';
}
