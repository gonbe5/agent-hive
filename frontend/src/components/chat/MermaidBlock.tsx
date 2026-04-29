import { useEffect, useRef, useState } from 'react';

/** 清理 SVG 字符串，移除潜在的 XSS 载体（script 标签和事件处理属性） */
function sanitizeSvg(svg: string): string {
  return svg
    // 移除 <script>...</script> 标签及其内容
    .replace(/<script[\s>][\s\S]*?<\/script>/gi, '')
    // 移除自闭合的 <script /> 标签
    .replace(/<script[^>]*\/>/gi, '')
    // 移除所有 on* 事件处理属性（如 onclick、onerror 等）
    .replace(/\s+on\w+\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]+)/gi, '');
}

let _counter = 0;
let _mermaidInit = false;

interface Props {
  code: string;
}

export function MermaidBlock({ code }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [error, setError] = useState<string | null>(null);
  const idRef = useRef(`mermaid-${++_counter}`);

  useEffect(() => {
    if (!containerRef.current) return;
    setError(null);

    // 懒加载 mermaid，避免将 2.5MB 的包打入主 bundle
    import('mermaid').then(({ default: mermaid }) => {
      if (!_mermaidInit) {
        mermaid.initialize({
          startOnLoad: false,
          theme: 'default',
          // 使用 strict 模式禁止图表中执行脚本，防止 XSS 攻击
          securityLevel: 'strict',
        });
        _mermaidInit = true;
      }
      return mermaid.render(idRef.current, code);
    })
      .then(({ svg }) => {
        if (containerRef.current) {
          // 对 SVG 输出进行清理，防止 XSS 注入
          containerRef.current.innerHTML = sanitizeSvg(svg);
        }
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err));
      });
  }, [code]);

  if (error) {
    return (
      <pre className="text-red-500 text-xs p-3 bg-red-50 dark:bg-red-900/20 rounded-lg border border-red-200 dark:border-red-800">
        Mermaid 渲染失败：{error}
      </pre>
    );
  }

  return (
    <div
      ref={containerRef}
      className="mermaid-container flex justify-center py-4 overflow-x-auto"
    />
  );
}
