/**
 * Phase 1 Day 2 — Streamdown 5 场景验收矩阵（契约测试）
 *
 * 目的：锁定 Day 2 plugin cleanup 后的 API 形状——源码不再显式传
 * remarkPlugins/rehypePlugins，改为 streamdown 内置默认 + `plugins.math` + `allowedTags`。
 *
 * Day 2 覆盖配置 = 业务源码（MessageBubble / ArtifactCard / MarkdownRenderer）实际使用的配置：
 *   plugins={{ math: MATH_PLUGIN }}   // 内置 katex（替代 rehypeKatex + remarkMath 显式传入）
 *   allowedTags={ALLOWED_TAGS}        // 扩展 rehype-sanitize defaultSchema，支持 KaTeX 元素
 *   （不传 remarkPlugins / rehypePlugins → streamdown 用内置 harden+raw+sanitize+gfm）
 *
 * Day 2 相对 Day 3 的契约变更：
 *   Scene 4 KaTeX `.katex` class 从 `.toBeNull()`（既存 bug）反转为 `.not.toBeNull()`
 *   （ALLOWED_TAGS 白名单了 span.className + mathml tags，bug 修复）
 *   删除 ReactMarkdown 对照组（react-markdown 在 Day 2 末卸载）
 */

import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/react';
import { Streamdown } from 'streamdown';
import { MATH_PLUGIN, ALLOWED_TAGS } from '../../../utils/streamdownConfig';

describe('Streamdown × Day 2 plugin config — 5 scene matrix', () => {
  it('scene 1: streaming incomplete fence renders code-block skeleton (shiki async)', () => {
    const incomplete = '```python\nprint("hello"';
    const { container } = render(
      <Streamdown
        parseIncompleteMarkdown
        plugins={{ math: MATH_PLUGIN }}
        allowedTags={ALLOWED_TAGS}
      >
        {incomplete}
      </Streamdown>
    );
    const codeBlock = container.querySelector('[data-streamdown="code-block"]');
    expect(codeBlock).not.toBeNull();
    expect(codeBlock?.getAttribute('data-language')).toBe('python');
  });

  it('scene 2: fenced code block renders data-streamdown="code-block" container', () => {
    const md = '```js\nconsole.log(42);\n```';
    const { container } = render(
      <Streamdown plugins={{ math: MATH_PLUGIN }} allowedTags={ALLOWED_TAGS}>
        {md}
      </Streamdown>
    );
    const codeBlock = container.querySelector('[data-streamdown="code-block"]');
    expect(codeBlock).not.toBeNull();
    expect(codeBlock?.getAttribute('data-language')).toBe('js');
    const header = codeBlock?.querySelector('[data-streamdown="code-block-header"]');
    expect(header).not.toBeNull();
    expect(header?.textContent?.trim()).toBe('js');
  });

  it('scene 3: inline code renders <code> without <pre>', () => {
    const md = 'call the `foo()` helper';
    const { container } = render(
      <Streamdown plugins={{ math: MATH_PLUGIN }} allowedTags={ALLOWED_TAGS}>
        {md}
      </Streamdown>
    );
    const paragraph = container.querySelector('p');
    expect(paragraph).not.toBeNull();
    const inlineCode = paragraph?.querySelector('code');
    expect(inlineCode).not.toBeNull();
    expect(inlineCode?.closest('pre')).toBeNull();
    expect(inlineCode?.textContent).toBe('foo()');
  });

  it('scene 4: KaTeX renders with .katex class preserved (Day 2 fix reverses Day 3 bug)', () => {
    const md = 'Einstein said $E = mc^2$ famously.';
    const { container } = render(
      <Streamdown plugins={{ math: MATH_PLUGIN }} allowedTags={ALLOWED_TAGS}>
        {md}
      </Streamdown>
    );
    expect(container.textContent).toMatch(/E\s*=\s*mc/);
    expect(container.querySelector('.katex')).not.toBeNull();
  });

  it('scene 5: GFM table renders <table> with <thead>/<tbody>', () => {
    const md = [
      '| name | age |',
      '|------|-----|',
      '| alice | 30 |',
      '| bob   | 25 |',
    ].join('\n');
    const { container } = render(
      <Streamdown plugins={{ math: MATH_PLUGIN }} allowedTags={ALLOWED_TAGS}>
        {md}
      </Streamdown>
    );
    const table = container.querySelector('table');
    expect(table).not.toBeNull();
    expect(table?.querySelector('thead')).not.toBeNull();
    expect(table?.querySelector('tbody')).not.toBeNull();
    expect(container.textContent).toContain('alice');
    expect(container.textContent).toContain('bob');
  });
});
