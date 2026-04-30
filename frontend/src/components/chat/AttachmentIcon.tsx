import { FileText, Image as ImageIcon, Music, Video, File } from 'lucide-react';

interface AttachmentIconProps {
  mimeType: string;
}

export function AttachmentIcon({ mimeType }: AttachmentIconProps) {
  if (mimeType.startsWith('image/')) {
    return <ImageIcon className="w-3.5 h-3.5 text-green-500" />;
  }
  if (mimeType.startsWith('audio/')) {
    return <Music className="w-3.5 h-3.5 text-[var(--text-secondary)]" />;
  }
  if (mimeType.startsWith('video/')) {
    return <Video className="w-3.5 h-3.5 text-red-500" />;
  }
  if (mimeType === 'application/pdf' || mimeType.includes('document') || mimeType.includes('word')) {
    return <FileText className="w-3.5 h-3.5 text-[var(--accent-600)] dark:text-[var(--accent-300)]" />;
  }
  if (
    mimeType.startsWith('text/') ||
    mimeType === 'application/json' ||
    mimeType === 'application/xml' ||
    mimeType === 'application/yaml'
  ) {
    return <FileText className="w-3.5 h-3.5 text-[var(--accent-500)]" />;
  }
  return <File className="w-3.5 h-3.5 text-[var(--text-secondary)]" />;
}
