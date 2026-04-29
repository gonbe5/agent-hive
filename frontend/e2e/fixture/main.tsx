import React from 'react';
import { createRoot } from 'react-dom/client';
import { Streamdown } from 'streamdown';
import { SHIKI_PLUGIN } from '../../src/utils/shikiPlugin';
import { STREAMDOWN_PLUGINS, ALLOWED_TAGS } from '../../src/utils/streamdownConfig';
import '../../src/i18n';

const pluginsCodeOnly = { code: SHIKI_PLUGIN } as const;

const PYTHON = '```python\ndef hello(name: str) -> str:\n    return f"Hello, {name}!"\n\nprint(hello("world"))\n```';
const JS = '```js\nconst sum = (a, b) => a + b;\nconsole.log(sum(2, 3));\n```';
const INCOMPLETE = '```rust\nfn main() {\n    println!("hello"';

// L2 真生产配置：走 STREAMDOWN_PLUGINS（含 BusinessCodeRenderer + SHIKI_PLUGIN + MATH_PLUGIN）
const PROD_PY = '```python\n# business renderer wraps shiki\nclass Agent:\n    def __init__(self, name: str):\n        self.name = name\n\n    def greet(self) -> str:\n        return f"Hi from {self.name}"\n```';
const PROD_TS = '```typescript\ninterface Task {\n  id: number;\n  done: boolean;\n}\n\nconst filterDone = (tasks: Task[]): Task[] =>\n  tasks.filter((t) => t.done);\n```';
const PROD_MATH = 'Inline $E = mc^2$ and display:\n\n$$\n\\int_0^\\infty e^{-x^2} dx = \\frac{\\sqrt{\\pi}}{2}\n$$';

function App() {
  return (
    <div>
      <h1 data-testid="title">Streamdown Shiki E2E Fixture</h1>

      {/* L1 pure plugin */}
      <section data-testid="python-section">
        <h2>Python complete (pure plugin)</h2>
        <Streamdown plugins={pluginsCodeOnly}>{PYTHON}</Streamdown>
      </section>
      <section data-testid="js-section">
        <h2>JavaScript complete (pure plugin)</h2>
        <Streamdown plugins={pluginsCodeOnly}>{JS}</Streamdown>
      </section>
      <section data-testid="rust-incomplete-section">
        <h2>Rust incomplete (pure plugin, streaming)</h2>
        <Streamdown plugins={pluginsCodeOnly} parseIncompleteMarkdown>{INCOMPLETE}</Streamdown>
      </section>

      {/* L2 生产配置：STREAMDOWN_PLUGINS 全量，含 BusinessCodeRenderer */}
      <section data-testid="prod-python-section">
        <h2>Python — STREAMDOWN_PLUGINS (BusinessCodeRenderer + shiki)</h2>
        <Streamdown plugins={STREAMDOWN_PLUGINS} allowedTags={ALLOWED_TAGS}>{PROD_PY}</Streamdown>
      </section>
      <section data-testid="prod-ts-section">
        <h2>TypeScript — STREAMDOWN_PLUGINS (BusinessCodeRenderer + shiki)</h2>
        <Streamdown plugins={STREAMDOWN_PLUGINS} allowedTags={ALLOWED_TAGS}>{PROD_TS}</Streamdown>
      </section>
      <section data-testid="prod-math-section">
        <h2>Math — STREAMDOWN_PLUGINS (katex)</h2>
        <Streamdown plugins={STREAMDOWN_PLUGINS} allowedTags={ALLOWED_TAGS}>{PROD_MATH}</Streamdown>
      </section>
    </div>
  );
}

const root = document.getElementById('root');
if (root) {
  createRoot(root).render(<App />);
}
