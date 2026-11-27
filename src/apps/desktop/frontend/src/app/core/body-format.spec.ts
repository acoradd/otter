import { bodyFormat, sniff } from './body-format';

describe('bodyFormat', () => {
  it('trusts the Content-Type when it says', () => {
    expect(bodyFormat('application/json', '<html>')).toEqual({ format: 'json', detected: false });
    expect(bodyFormat('application/problem+json', '')).toEqual({ format: 'json', detected: false });
    expect(bodyFormat('text/html', '{}')).toEqual({ format: 'html', detected: false });
    expect(bodyFormat('application/soap+xml', '')).toEqual({ format: 'xml', detected: false });
  });

  it('looks at the content when the Content-Type is missing or vague', () => {
    expect(bodyFormat('', '  {"a": 1}')).toEqual({ format: 'json', detected: true });
    expect(bodyFormat('text/plain', '<!DOCTYPE html><html></html>')).toEqual({
      format: 'html',
      detected: true,
    });
    expect(bodyFormat('application/octet-stream', '<?xml version="1.0"?><a/>')).toEqual({
      format: 'xml',
      detected: true,
    });
    expect(bodyFormat('text/plain', 'hello')).toEqual({ format: 'plaintext', detected: false });
  });
});

describe('sniff', () => {
  it.each([
    ['[1, 2]', 'json'],
    ['{"broken": ', 'plaintext'],
    ['{{name}} is not JSON', 'plaintext'],
    ['<html lang="fr"><body></body></html>', 'html'],
    ['<div class="x"><p>Hi</p></div>', 'html'],
    ['<note><to>Ada</to></note>', 'xml'],
    ['<item id="1"/>', 'xml'],
    ['< 3 apples', 'plaintext'],
    ['<not closed', 'plaintext'],
    ['', 'plaintext'],
  ])('%s → %s', (body, want) => {
    expect(sniff(body)).toBe(want);
  });
});
