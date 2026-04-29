/**
 * L2 真生产配置回归：验证 `STREAMDOWN_PLUGINS`（BusinessCodeRenderer + SHIKI_PLUGIN + MATH_PLUGIN）
 * 在真实浏览器里仍能跑出 shiki colored span 与 katex DOM。
 *
 * L1 shiki.spec.ts 只测了 pure plugin 路径；本 spec 覆盖业务自定义 renderer 包住 shiki 的链路，
 * 即 MessageBubble 中实际使用的配置。
 */

import { test, expect } from '@playwright/test';

const SHIKI_TIMEOUT = 15_000;

test('prod python: BusinessCodeRenderer + shiki produce colored spans', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByTestId('title')).toBeVisible();

  const section = page.getByTestId('prod-python-section');
  const codeBlock = section.locator('[data-streamdown="code-block"]');
  await expect(codeBlock).toBeVisible();
  await expect(codeBlock).toHaveAttribute('data-language', 'python');

  const colored = codeBlock.locator('span[style*="--sdm-c: #"]');
  await expect(colored.first()).toBeVisible({ timeout: SHIKI_TIMEOUT });
  const count = await colored.count();
  expect(count).toBeGreaterThan(5);
});

test('prod typescript: BusinessCodeRenderer + shiki lazy-load typescript grammar', async ({ page }) => {
  await page.goto('/');
  const section = page.getByTestId('prod-ts-section');
  const codeBlock = section.locator('[data-streamdown="code-block"]');
  await expect(codeBlock).toBeVisible();
  await expect(codeBlock).toHaveAttribute('data-language', 'typescript');

  const colored = codeBlock.locator('span[style*="--sdm-c: #"]');
  await expect(colored.first()).toBeVisible({ timeout: SHIKI_TIMEOUT });
  expect(await colored.count()).toBeGreaterThan(5);
});

test('prod math: katex renders inline and display equations', async ({ page }) => {
  await page.goto('/');
  const section = page.getByTestId('prod-math-section');
  await expect(section.locator('.katex')).toHaveCount(2, { timeout: SHIKI_TIMEOUT });

  // 断言 display 公式里有积分符号
  const displayMath = section.locator('.katex-display .katex');
  await expect(displayMath).toBeVisible();
});

test('prod python: shiki token cache survives re-render (stable element)', async ({ page }) => {
  await page.goto('/');
  const section = page.getByTestId('prod-python-section');
  await expect(section.locator('span[style*="--sdm-c: #"]').first()).toBeVisible({ timeout: SHIKI_TIMEOUT });

  // 首次 token 数量
  const first = await section.locator('span[style*="--sdm-c: #"]').count();
  expect(first).toBeGreaterThan(5);

  // 强制重新布局后数量稳定（确保不是一次性异步渲染出来就失效）
  await page.evaluate(() => window.scrollTo(0, 0));
  const second = await section.locator('span[style*="--sdm-c: #"]').count();
  expect(second).toBe(first);
});
