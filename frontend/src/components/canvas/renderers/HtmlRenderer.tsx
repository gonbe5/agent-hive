import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Shield, ShieldOff } from 'lucide-react';

interface Props {
  content: string;
}

export function HtmlRenderer({ content }: Props) {
  const { t } = useTranslation();
  const [scriptsEnabled, setScriptsEnabled] = useState(false);

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-end px-2 py-1 border-b border-[var(--border-color)] bg-[var(--bg-card)] shrink-0">
        <button
          onClick={() => setScriptsEnabled(!scriptsEnabled)}
          className={`flex items-center gap-1 px-2 py-1 rounded-md text-xs transition-colors ${
            scriptsEnabled
              /* warning semantic */
              ? 'text-[var(--warning)] dark:text-[var(--warning)] bg-[var(--warning)]/10 dark:bg-[var(--warning)]/10'
              : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-hover)]'
          }`}
          title={scriptsEnabled ? t('canvas.disableScripts') : t('canvas.enableScripts')}
        >
          {scriptsEnabled ? <ShieldOff className="w-3.5 h-3.5" /> : <Shield className="w-3.5 h-3.5" />}
          <span>{scriptsEnabled ? t('canvas.disableScripts') : t('canvas.enableScripts')}</span>
        </button>
      </div>
      <iframe
        srcDoc={content}
        sandbox={scriptsEnabled ? 'allow-scripts' : ''}
        className="canvas-iframe flex-1"
        title="HTML Preview"
      />
    </div>
  );
}
