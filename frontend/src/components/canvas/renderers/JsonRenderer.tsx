import { useState, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { ChevronRight, ChevronDown } from 'lucide-react';

interface Props {
  content: string;
}

export function JsonRenderer({ content }: Props) {
  const { t } = useTranslation();
  let parsed: unknown;
  try {
    parsed = JSON.parse(content);
  } catch {
    return (
      <pre className="p-4 font-mono text-sm text-red-500 overflow-auto h-full">
        {t('canvas.jsonParseError')}: {content}
      </pre>
    );
  }

  return (
    <div className="p-4 font-mono text-[13px] leading-[1.6] overflow-auto h-full">
      <JsonNode value={parsed} />
    </div>
  );
}

function JsonNode({ value, name }: { value: unknown; name?: string }) {
  const [expanded, setExpanded] = useState(true);
  const toggle = useCallback(() => setExpanded((v) => !v), []);

  const label = name !== undefined ? (
    <span className="json-tree-key">{`"${name}": `}</span>
  ) : null;

  if (value === null) {
    return <div className="ml-4">{label}<span className="json-tree-null">null</span></div>;
  }
  if (typeof value === 'boolean') {
    return <div className="ml-4">{label}<span className="json-tree-boolean">{String(value)}</span></div>;
  }
  if (typeof value === 'number') {
    return <div className="ml-4">{label}<span className="json-tree-number">{value}</span></div>;
  }
  if (typeof value === 'string') {
    return <div className="ml-4">{label}<span className="json-tree-string">"{value}"</span></div>;
  }

  if (Array.isArray(value)) {
    if (value.length === 0) {
      return <div className="ml-4">{label}{'[]'}</div>;
    }
    return (
      <div className="ml-4">
        <span className="cursor-pointer inline-flex items-center gap-0.5" onClick={toggle}>
          {expanded ? <ChevronDown className="w-3 h-3 inline" /> : <ChevronRight className="w-3 h-3 inline" />}
          {label}{'['}
          {!expanded && <span className="text-[var(--text-secondary)]"> {value.length} items ]</span>}
        </span>
        {expanded && (
          <>
            {value.map((item, i) => (
              <JsonNode key={i} value={item} />
            ))}
            <div className="ml-4">{']'}</div>
          </>
        )}
      </div>
    );
  }

  if (typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>);
    if (entries.length === 0) {
      return <div className="ml-4">{label}{'{}'}</div>;
    }
    return (
      <div className="ml-4">
        <span className="cursor-pointer inline-flex items-center gap-0.5" onClick={toggle}>
          {expanded ? <ChevronDown className="w-3 h-3 inline" /> : <ChevronRight className="w-3 h-3 inline" />}
          {label}{'{'}
          {!expanded && <span className="text-[var(--text-secondary)]"> {entries.length} keys {'}'}</span>}
        </span>
        {expanded && (
          <>
            {entries.map(([k, v]) => (
              <JsonNode key={k} value={v} name={k} />
            ))}
            <div className="ml-4">{'}'}</div>
          </>
        )}
      </div>
    );
  }

  return <div className="ml-4">{label}{String(value)}</div>;
}
