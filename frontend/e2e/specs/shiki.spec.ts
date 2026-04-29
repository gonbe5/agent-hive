/**
 * Phase 1 Day 3 — Playwright e2e: streamdown shiki 真实浏览器验证
 *
 * jsdom 不执行 dynamic import() 的 wasm/worker，Suspense 永远停在 skeleton 状态——
 * 所以 vitest 矩阵只能验证 `data-streamdown="code-block"` 容器与语言 attr，
 * shiki 真机是否完成 grammar+theme 加载、是否生成带 inline color style 的 token span，
 * 必须靠真实浏览器跑一次。
 *
 * 本 spec 只验 3 个不变量（不覆盖业务 UI）：
 *   1. Python 代码块：shiki 异步加载完成后，code-block 容器内出现 inline `style="color:..."` 的 span
 *   2. JS 代码块：同上（不同语言，确认 shiki grammar 懒加载逻辑跨语言都工作）
 *   3. 流式未闭合 fence：shiki 不会炸，容器依然渲染（有 skeleton 或部分 token）
 */

import { test, expect } from '@playwright/test';

const SHIKI_TIMEOUT = 15_000;

test('python code block: shiki async tokenization produces colored spans', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByTestId('title')).toBeVisible();

  const section = page.getByTestId('python-section');
  const codeBlock = section.locator('[data-streamdown="code-block"]');
  await expect(codeBlock).toBeVisible();
  await expect(codeBlock).toHaveAttribute('data-language', 'python');

  // shiki 完成后会注入带 inline color 的 span token（每个 token 一个 span）
  const colored = codeBlock.locator('span[style*="--sdm-c: #"]');
  await expect(colored.first()).toBeVisible({ timeout: SHIKI_TIMEOUT });

  const count = await colored.count();
  expect(count).toBeGreaterThan(5); // 至少 5 个 token（def/hello/name/str/return...）
});

test('javascript code block: shiki loads a different grammar lazily', async ({ page }) => {
  await page.goto('/');
  const section = page.getByTestId('js-section');
  const codeBlock = section.locator('[data-streamdown="code-block"]');
  await expect(codeBlock).toBeVisible();
  await expect(codeBlock).toHaveAttribute('data-language', 'js');

  const colored = codeBlock.locator('span[style*="--sdm-c: #"]');
  await expect(colored.first()).toBeVisible({ timeout: SHIKI_TIMEOUT });
  expect(await colored.count()).toBeGreaterThan(3);
});

test('streaming incomplete fence: container renders without crashing', async ({ page }) => {
  await page.goto('/');
  const section = page.getByTestId('rust-incomplete-section');
  const codeBlock = section.locator('[data-streamdown="code-block"]');
  await expect(codeBlock).toBeVisible();
  await expect(codeBlock).toHaveAttribute('data-language', 'rust');

  // 即使未闭合，只要 parseIncompleteMarkdown 生效，shiki 也应该 tokenize 已有部分
  const colored = codeBlock.locator('span[style*="--sdm-c: #"]');
  await expect(colored.first()).toBeVisible({ timeout: SHIKI_TIMEOUT });
});
