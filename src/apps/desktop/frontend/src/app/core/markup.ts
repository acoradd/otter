/** Elements with no closing tag in HTML. */
const VOID = new Set([
  'area',
  'base',
  'br',
  'col',
  'embed',
  'hr',
  'img',
  'input',
  'link',
  'meta',
  'param',
  'source',
  'track',
  'wbr',
]);
/** HTML elements whose content is kept exactly as it is. */
const RAW = new Set(['script', 'style', 'pre', 'textarea']);
/** Text up to this length stays on the line of its element: <title>Page</title>. */
const INLINE_TEXT = 80;

type Token =
  | { kind: 'open'; text: string; name: string }
  | { kind: 'close'; text: string; name: string }
  | { kind: 'single'; text: string } // void, self-closing, comment, doctype, <?…?>, CDATA
  | { kind: 'text'; text: string }
  | { kind: 'raw'; text: string }; // the whole of a <script>, <style>, <pre>, <textarea>

/** The end of a tag starting at i (index after '>'), skipping quoted attribute values. */
function tagEnd(s: string, i: number): number {
  let quote = '';
  for (let j = i + 1; j < s.length; j++) {
    const c = s[j];
    if (quote) {
      if (c === quote) {
        quote = '';
      }
    } else if (c === '"' || c === "'") {
      quote = c;
    } else if (c === '>') {
      return j + 1;
    }
  }
  return s.length;
}

function tagName(tag: string): string {
  return /^<\/?\s*([^\s/>]+)/.exec(tag)?.[1].toLowerCase() ?? '';
}

function tokenize(s: string, html: boolean): Token[] {
  const tokens: Token[] = [];
  let i = 0;
  while (i < s.length) {
    if (s[i] !== '<') {
      const next = s.indexOf('<', i);
      const end = next < 0 ? s.length : next;
      tokens.push({ kind: 'text', text: s.slice(i, end) });
      i = end;
      continue;
    }
    for (const [start, stop] of [
      ['<!--', '-->'],
      ['<![CDATA[', ']]>'],
    ]) {
      if (s.startsWith(start, i)) {
        const end = s.indexOf(stop, i + start.length);
        const to = end < 0 ? s.length : end + stop.length;
        tokens.push({ kind: 'single', text: s.slice(i, to) });
        i = to;
        break;
      }
    }
    if (i < s.length && s[i] === '<' && !s.startsWith('<!--', i) && !s.startsWith('<![CDATA[', i)) {
      const end = tagEnd(s, i);
      const tag = s.slice(i, end);
      const name = tagName(tag);
      i = end;
      if (tag.startsWith('</')) {
        tokens.push({ kind: 'close', text: tag, name });
      } else if (
        tag.startsWith('<!') ||
        tag.startsWith('<?') ||
        tag.endsWith('/>') ||
        (html && VOID.has(name))
      ) {
        tokens.push({ kind: 'single', text: tag });
      } else if (html && RAW.has(name)) {
        // Everything up to the matching close tag, untouched.
        const close = new RegExp(`</\\s*${name}\\s*>`, 'i').exec(s.slice(i));
        const to = close ? i + close.index + close[0].length : s.length;
        tokens.push({ kind: 'raw', text: tag + s.slice(i, to) });
        i = to;
      } else if (name) {
        tokens.push({ kind: 'open', text: tag, name });
      } else {
        tokens.push({ kind: 'text', text: tag }); // a lone '<'
      }
    }
  }
  return tokens;
}

/**
 * Indents HTML or XML for reading: one element per line, two spaces per
 * level, short text on its element's line. Comments, doctype, CDATA and the
 * content of <script>, <style>, <pre> and <textarea> are kept as they are.
 * Unbalanced markup is still indented as well as it can be, nothing dropped.
 */
export function prettyMarkup(source: string, html: boolean): string {
  const tokens = tokenize(source.trim(), html).filter(
    (t) => t.kind !== 'text' || t.text.trim() !== '',
  );
  const lines: string[] = [];
  let depth = 0;
  const pad = () => '  '.repeat(Math.max(0, depth));
  const squash = (text: string) => text.trim().replace(/\s+/g, ' ');

  for (let i = 0; i < tokens.length; i++) {
    const t = tokens[i];
    switch (t.kind) {
      case 'open': {
        const next = tokens[i + 1];
        const after = tokens[i + 2];
        // <p></p> and <title>Short</title> stay on one line.
        if (next?.kind === 'close' && next.name === t.name) {
          lines.push(pad() + t.text + next.text);
          i += 1;
        } else if (
          next?.kind === 'text' &&
          after?.kind === 'close' &&
          after.name === t.name &&
          squash(next.text).length <= INLINE_TEXT
        ) {
          lines.push(pad() + t.text + squash(next.text) + after.text);
          i += 2;
        } else {
          lines.push(pad() + t.text);
          depth++;
        }
        break;
      }
      case 'close':
        depth--;
        lines.push(pad() + t.text);
        break;
      case 'text':
        lines.push(pad() + squash(t.text));
        break;
      case 'single':
      case 'raw':
        lines.push(pad() + t.text);
        break;
    }
  }
  return lines.join('\n');
}
