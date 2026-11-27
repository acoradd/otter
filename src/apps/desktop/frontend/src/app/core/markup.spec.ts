import { prettyMarkup } from './markup';

describe('prettyMarkup', () => {
  it('indents minified HTML, one element per line', () => {
    const html =
      '<!DOCTYPE html><html><head><meta charset="utf-8"><title>Otter</title></head>' +
      '<body><div class="card"><h1>Hello</h1><p>Some <b>bold</b> text</p><br><img src="a.png" alt="x"></div></body></html>';
    expect(prettyMarkup(html, true)).toBe(
      [
        '<!DOCTYPE html>',
        '<html>',
        '  <head>',
        '    <meta charset="utf-8">',
        '    <title>Otter</title>',
        '  </head>',
        '  <body>',
        '    <div class="card">',
        '      <h1>Hello</h1>',
        '      <p>',
        '        Some',
        '        <b>bold</b>',
        '        text',
        '      </p>',
        '      <br>',
        '      <img src="a.png" alt="x">',
        '    </div>',
        '  </body>',
        '</html>',
      ].join('\n'),
    );
  });

  it('keeps scripts, styles, pre and comments as they are', () => {
    const html =
      '<div><script>if (a < b) { x = "</div>"; }</script><pre>  keep\n   this</pre><!-- a > b --></div>';
    expect(prettyMarkup(html, true)).toBe(
      [
        '<div>',
        '  <script>if (a < b) { x = "</div>"; }</script>',
        '  <pre>  keep\n   this</pre>',
        '  <!-- a > b -->',
        '</div>',
      ].join('\n'),
    );
  });

  it('reads attributes holding ">" and quotes', () => {
    expect(prettyMarkup(`<a title="1 > 0" data-x='"q"'>link</a>`, true)).toBe(
      `<a title="1 > 0" data-x='"q"'>link</a>`,
    );
  });

  it('indents XML: declarations, self-closing tags, CDATA, namespaces', () => {
    const xml =
      '<?xml version="1.0"?><soap:Envelope xmlns:soap="x"><soap:Body><item id="1"/><note><![CDATA[<raw>]]></note><empty></empty></soap:Body></soap:Envelope>';
    expect(prettyMarkup(xml, false)).toBe(
      [
        '<?xml version="1.0"?>',
        '<soap:Envelope xmlns:soap="x">',
        '  <soap:Body>',
        '    <item id="1"/>',
        '    <note>',
        '      <![CDATA[<raw>]]>',
        '    </note>',
        '    <empty></empty>',
        '  </soap:Body>',
        '</soap:Envelope>',
      ].join('\n'),
    );
  });

  it('in XML, <br> is an element like any other', () => {
    expect(prettyMarkup('<a><br>x</br></a>', false)).toBe('<a>\n  <br>x</br>\n</a>');
  });

  it('copes with unbalanced HTML without dropping anything', () => {
    const out = prettyMarkup('<ul><li>one<li>two</ul></div>text', true);
    expect(out.replace(/\s+/g, '')).toBe('<ul><li>one<li>two</ul></div>text');
  });

  it('keeps long text on its own line', () => {
    const long = 'x'.repeat(90);
    expect(prettyMarkup(`<p>${long}</p>`, true)).toBe(`<p>\n  ${long}\n</p>`);
  });
});
