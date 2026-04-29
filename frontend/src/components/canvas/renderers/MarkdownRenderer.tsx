import { Streamdown } from 'streamdown';
import { MATH_PLUGIN, ALLOWED_TAGS } from '../../../utils/streamdownConfig';

interface Props {
  content: string;
}

export function MarkdownRenderer({ content }: Props) {
  return (
    <div
      className="prose prose-sm max-w-none dark:prose-invert prose-headings:text-[var(--text-primary)] prose-p:text-[var(--text-primary)] prose-li:text-[var(--text-primary)] prose-strong:text-[var(--text-primary)] prose-a:text-[var(--accent)] dark:prose-a:text-[var(--accent)] text-[var(--text-primary)] text-[13px] leading-[1.6] p-5 overflow-auto h-full prose-p:my-2 prose-headings:mb-2 prose-headings:mt-4 prose-li:my-0.5 prose-pre:my-2 prose-blockquote:my-2"
    >
      <Streamdown plugins={{ math: MATH_PLUGIN }} allowedTags={ALLOWED_TAGS}>
        {content}
      </Streamdown>
    </div>
  );
}
