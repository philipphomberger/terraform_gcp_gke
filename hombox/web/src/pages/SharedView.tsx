import { useState, useEffect, useCallback, useRef } from 'react';
import { useParams } from 'react-router-dom';
import { api, type FileEntry, type Share } from '../lib/api';
import { File, Folder, Upload, Download } from 'lucide-react';

export default function SharedView() {
  const { token } = useParams<{ token: string }>();
  const [share, setShare] = useState<Share | null>(null);
  const [files, setFiles] = useState<FileEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [uploading, setUploading] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const loadShare = useCallback(async () => {
    if (!token) return;
    setLoading(true);
    setError('');
    try {
      const data = await api.getShare(token);
      setShare(data.share);
      setFiles(data.files);
    } catch (err: any) {
      setError(err.message || 'Invalid or expired link');
    } finally {
      setLoading(false);
    }
  }, [token]);

  useEffect(() => { loadShare(); }, [loadShare]);

  const handleUpload = async (files: FileList | null) => {
    if (!files || !token) return;
    for (const f of Array.from(files)) {
      setUploading(true);
      try {
        const buf = await f.arrayBuffer();
        const hash = Array.from(new Uint8Array(buf))
          .map((b) => b.toString(16).padStart(2, '0'))
          .join('')
          .slice(0, 64);

        const { file_id, signed_url } = await api.anonInitUpload(
          token,
          f.name,
          f.size,
          f.type || 'application/octet-stream'
        );

        await fetch(signed_url, { method: 'PUT', body: f });
        await api.anonConfirmUpload(token, file_id, hash);
        await loadShare();
      } catch (err) {
        console.error('Upload failed', err);
      } finally {
        setUploading(false);
      }
    }
  };

  const isImage = (f: FileEntry) => f.mime_type?.startsWith('image/');

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="animate-spin w-8 h-8 border-4 border-blue-600 border-t-transparent rounded-full" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="bg-white p-8 rounded-xl shadow-sm border border-gray-200 text-center max-w-md">
          <div className="text-red-500 text-5xl mb-4">!</div>
          <h2 className="text-xl font-bold text-gray-900 mb-2">Link Unavailable</h2>
          <p className="text-gray-500">{error}</p>
        </div>
      </div>
    );
  }

  const canWrite = share?.permissions?.write === true;

  return (
    <div className="min-h-screen bg-gray-50">
      <div className="max-w-3xl mx-auto p-6">
        <div className="bg-white rounded-xl border border-gray-200 p-6 mb-6">
          <div className="flex items-center gap-2 mb-2">
            <Folder className="w-6 h-6 text-blue-600" />
            <div>
              <h1 className="text-xl font-bold text-gray-900">Shared Folder</h1>
              <p className="text-sm text-gray-500">
                {canWrite ? 'You can view and upload files' : 'View-only access'}
              </p>
            </div>
          </div>
        </div>

        {canWrite && (
          <div className="mb-4">
            <button
              onClick={() => fileInputRef.current?.click()}
              disabled={uploading}
              className="flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg text-sm font-medium hover:bg-blue-700 disabled:opacity-50 cursor-pointer border-none"
            >
              <Upload className="w-4 h-4" />
              {uploading ? 'Uploading...' : 'Upload File'}
            </button>
            <input
              ref={fileInputRef}
              type="file"
              className="hidden"
              multiple
              onChange={(e) => handleUpload(e.target.files)}
            />
          </div>
        )}

        {files.length === 0 ? (
          <div className="bg-white rounded-xl border border-gray-200 p-12 text-center">
            <p className="text-gray-400">No files in this folder</p>
          </div>
        ) : (
          <div className="bg-white rounded-xl border border-gray-200 divide-y divide-gray-100">
            {files.map((f) => (
              <div key={f.id} className="flex items-center gap-3 px-4 py-3">
                {f.is_folder ? (
                  <Folder className="w-5 h-5 text-blue-600 flex-shrink-0" />
                ) : isImage(f) ? (
                  <img
                    src={token ? api.anonDownloadUrl(token, f.id) : ''}
                    className="w-5 h-5 rounded object-cover flex-shrink-0"
                    alt=""
                  />
                ) : (
                  <File className="w-5 h-5 text-gray-400 flex-shrink-0" />
                )}

                <span className="flex-1 text-sm font-medium text-gray-900 truncate">
                  {f.name}
                </span>

                <span className="text-xs text-gray-400">{formatSize(f.size)}</span>

                <a
                  href={token ? api.anonDownloadUrl(token, f.id) : '#'}
                  className="flex items-center gap-1 px-2 py-1 text-xs text-blue-600 hover:bg-blue-50 rounded no-underline"
                >
                  <Download className="w-3.5 h-3.5" />
                  Download
                </a>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

function formatSize(bytes: number): string {
  if (bytes === 0) return '';
  const units = ['B', 'KB', 'MB', 'GB'];
  let i = 0;
  let size = bytes;
  while (size >= 1024 && i < units.length - 1) {
    size /= 1024;
    i++;
  }
  return size.toFixed(i > 0 ? 1 : 0) + ' ' + units[i];
}
