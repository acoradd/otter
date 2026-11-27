// Loaded on demand (code-editor.ts): Monaco and its languages, workers and
// stylesheet stay out of the app's start.
import * as monaco from 'monaco-editor';

type Monaco = typeof monaco;

declare global {
  interface Window {
    MonacoEnvironment?: { getWorker(workerId: string, label: string): Worker };
  }
}

window.MonacoEnvironment = {
  getWorker(_: string, label: string): Worker {
    switch (label) {
      case 'json':
        return new Worker(new URL('./json.worker', import.meta.url), { type: 'module' });
      case 'typescript':
      case 'javascript':
        return new Worker(new URL('./ts.worker', import.meta.url), { type: 'module' });
      default:
        return new Worker(new URL('./editor.worker', import.meta.url), { type: 'module' });
    }
  },
};

/** Monaco's stylesheet, built apart (angular.json, bundle "monaco"). */
function loadStyles(): void {
  if (!document.querySelector('link[data-monaco]')) {
    const link = document.createElement('link');
    link.rel = 'stylesheet';
    link.href = 'monaco.css';
    link.dataset['monaco'] = '';
    document.head.appendChild(link);
  }
}

/** A color token of styles.scss, as Monaco wants it (#rrggbb). */
function token(name: string): string {
  const value = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  return /^#[0-9a-f]{6}$/i.test(value) ? value : '#000000';
}

/** The Otter themes, from the current design tokens. */
export function defineThemes(m: Monaco): void {
  const common = (dark: boolean): monaco.editor.IStandaloneThemeData => ({
    base: dark ? 'vs-dark' : 'vs',
    inherit: true,
    rules: [],
    colors: {
      'editor.background': token('--color-bg'),
      'editor.foreground': token('--color-text'),
      'editorLineNumber.foreground': token('--color-text-muted'),
      'editorCursor.foreground': token('--color-accent'),
    },
  });
  m.editor.defineTheme('otter-dark', common(true));
  m.editor.defineTheme('otter-light', common(false));
}

export function load(): Monaco {
  loadStyles();
  return monaco;
}
