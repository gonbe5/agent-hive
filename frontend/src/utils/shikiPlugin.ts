import type { CodeHighlighterPlugin, ThemeInput } from 'streamdown';
import { bundledLanguagesInfo, type BundledLanguage } from 'shiki';
import { highlightCode } from './shikiHighlight';

type HighlightResult = Parameters<
  Parameters<CodeHighlighterPlugin['highlight']>[1] & ((r: never) => void)
>[0];

const SUPPORTED_LANGS: BundledLanguage[] = bundledLanguagesInfo.map(
  (info) => info.id as BundledLanguage,
);
const SUPPORTED_SET = new Set<string>(SUPPORTED_LANGS);
const THEMES: [ThemeInput, ThemeInput] = ['github-light', 'github-dark'];

export const SHIKI_PLUGIN: CodeHighlighterPlugin = {
  name: 'shiki',
  type: 'code-highlighter',
  getThemes: () => THEMES,
  getSupportedLanguages: () => SUPPORTED_LANGS,
  supportsLanguage: (language) => SUPPORTED_SET.has(language),
  highlight: (options, callback) => {
    const wrapped = callback
      ? (t: { tokens: unknown[][]; bg: string; fg: string }) =>
          callback({
            tokens: t.tokens as HighlightResult['tokens'],
            bg: t.bg,
            fg: t.fg,
          })
      : undefined;
    const sync = highlightCode(
      options.code,
      options.language as BundledLanguage,
      wrapped,
    );
    if (!sync) return null;
    return {
      tokens: sync.tokens as HighlightResult['tokens'],
      bg: sync.bg,
      fg: sync.fg,
    };
  },
};
