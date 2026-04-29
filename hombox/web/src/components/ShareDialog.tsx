import { useState, useEffect } from 'react';
import { X, Copy, Check, Link, Globe } from 'lucide-react';
import { api, type FileEntry, type Share } from '../lib/api';

interface ShareDialogProps {
  file: FileEntry;
  onClose: () => void;
}

export default function ShareDialog({ file, onClose }: ShareDialogProps) {
  const [allowWrite, setAllowWrite] = useState(false);
  const [share, setShare] = useState<Share | null>(null);
  const [copied, setCopied] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    const handleEsc = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', handleEsc);
    return () => document.removeEventListener('keydown', handleEsc);
  }, [onClose]);

  const handleCreate = async () => {
    setLoading(true);
    setError('');
    try {
      const s = await api.createShare(file.id, 'anonymous', { read: true, write: allowWrite });
      setShare(s);
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const shareUrl = share?.token ? `${window.location.origin}/s/${share.token}` : '';

  const handleCopy = async () => {
    await navigator.clipboard.writeText(shareUrl);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative bg-white rounded-xl shadow-xl border border-gray-200 w-full max-w-md p-6">
        <button
          onClick={onClose}
          className="absolute top-4 right-4 p-1 hover:bg-gray-100 rounded bg-transparent border-none cursor-pointer"
        >
          <X className="w-4 h-4 text-gray-500" />
        </button>

        <div className="flex items-center gap-2 mb-4">
          <Globe className="w-5 h-5 text-blue-600" />
          <h3 className="text-lg font-semibold text-gray-900">Share "{file.name}"</h3>
        </div>

        {!share ? (
          <div className="flex flex-col gap-4">
            <label className="flex items-center gap-3 cursor-pointer">
              <input
                type="checkbox"
                checked={allowWrite}
                onChange={(e) => setAllowWrite(e.target.checked)}
                className="w-4 h-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
              />
              <div>
                <p className="text-sm font-medium text-gray-900">Allow uploads</p>
                <p className="text-xs text-gray-500">Anyone with the link can upload files to this folder</p>
              </div>
            </label>

            {error && <p className="text-sm text-red-600 bg-red-50 p-2 rounded">{error}</p>}

            <div className="flex gap-2">
              <button
                onClick={handleCreate}
                disabled={loading}
                className="flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg text-sm font-medium hover:bg-blue-700 disabled:opacity-50 cursor-pointer border-none"
              >
                <Link className="w-4 h-4" />
                {loading ? 'Creating...' : 'Create share link'}
              </button>
              <button
                onClick={onClose}
                className="px-4 py-2 border border-gray-300 rounded-lg text-sm hover:bg-gray-50 cursor-pointer bg-white"
              >
                Cancel
              </button>
            </div>
          </div>
        ) : (
          <div className="flex flex-col gap-4">
            <div>
              <p className="text-sm text-gray-600 mb-2">Share link created:</p>
              <div className="flex gap-2">
                <input
                  type="text"
                  value={shareUrl}
                  readOnly
                  className="flex-1 px-3 py-2 bg-gray-50 border border-gray-200 rounded-lg text-sm text-gray-700 focus:outline-none"
                  onFocus={(e) => e.target.select()}
                />
                <button
                  onClick={handleCopy}
                  className="flex items-center gap-1 px-3 py-2 bg-gray-100 hover:bg-gray-200 rounded-lg text-sm font-medium transition-colors cursor-pointer border-none"
                >
                  {copied ? <Check className="w-4 h-4 text-green-600" /> : <Copy className="w-4 h-4" />}
                  {copied ? 'Copied' : 'Copy'}
                </button>
              </div>
            </div>

            <div className="flex gap-2 text-xs text-gray-500">
              <span>Permissions: Read{allowWrite ? ' + Write' : ''}</span>
              {share.expires_at && <span>· Expires: {new Date(share.expires_at).toLocaleDateString()}</span>}
            </div>

            <button
              onClick={onClose}
              className="px-4 py-2 bg-gray-100 hover:bg-gray-200 rounded-lg text-sm cursor-pointer border-none"
            >
              Done
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
