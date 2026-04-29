import { useEffect, useState } from 'react';
import { X, Download } from 'lucide-react';
import { api, type FileEntry } from '../lib/api';

interface ImagePreviewProps {
  file: FileEntry;
  url: string;
  onClose: () => void;
}

export default function ImagePreview({ file, url, onClose }: ImagePreviewProps) {
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const handleEsc = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', handleEsc);
    return () => document.removeEventListener('keydown', handleEsc);
  }, [onClose]);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80">
      <div className="absolute top-4 right-4 flex items-center gap-2 z-10">
        <a
          href={api.getDownloadUrl(file.id)}
          className="flex items-center gap-1 px-3 py-1.5 bg-white/10 hover:bg-white/20 text-white rounded-lg text-sm backdrop-blur transition-colors no-underline"
          download
        >
          <Download className="w-4 h-4" />
        </a>
        <button
          onClick={onClose}
          className="p-1.5 bg-white/10 hover:bg-white/20 rounded-lg backdrop-blur transition-colors border-none cursor-pointer"
        >
          <X className="w-5 h-5 text-white" />
        </button>
      </div>

      <button
        onClick={onClose}
        className="absolute inset-0 border-none bg-transparent cursor-default"
      />

      <div className="relative max-w-[90vw] max-h-[90vh]">
        {loading && (
          <div className="absolute inset-0 flex items-center justify-center">
            <div className="w-8 h-8 border-4 border-white/30 border-t-white rounded-full animate-spin" />
          </div>
        )}
        <img
          src={url}
          alt={file.name}
          onLoad={() => setLoading(false)}
          className="max-w-[90vw] max-h-[90vh] object-contain rounded-lg"
        />
        <p className="absolute -bottom-8 left-0 right-0 text-center text-sm text-white/70 truncate">
          {file.name}
        </p>
      </div>
    </div>
  );
}
