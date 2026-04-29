import { useState, useEffect, useCallback, useRef } from 'react';
import { useParams, useNavigate, Link } from 'react-router-dom';
import { api, type FileEntry } from '../lib/api';
import FileUpload from '../components/FileUpload';
import ShareDialog from '../components/ShareDialog';
import ImagePreview from '../components/ImagePreview';
import { Folder, File, Plus, Trash2, Pencil, Share2, Home, ChevronRight, Search } from 'lucide-react';

export default function Dashboard() {
  const { folderId } = useParams();
  const navigate = useNavigate();
  const [files, setFiles] = useState<FileEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [newFolderName, setNewFolderName] = useState('');
  const [showNewFolder, setShowNewFolder] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editName, setEditName] = useState('');
  const [shareFile, setShareFile] = useState<FileEntry | null>(null);
  const [previewFile, setPreviewFile] = useState<FileEntry | null>(null);
  const [previewUrl, setPreviewUrl] = useState('');
  const [uploading, setUploading] = useState<string[]>([]);
  const [searchQuery, setSearchQuery] = useState('');
  const [isSearching, setIsSearching] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const loadFiles = useCallback(async () => {
    setLoading(true);
    try {
      let data: FileEntry[];
      if (isSearching && searchQuery) {
        data = await api.listFiles(); // search via dedicated endpoint
        data = data.filter(f => f.name.toLowerCase().includes(searchQuery.toLowerCase()));
      } else {
        data = await api.listFiles(folderId);
      }
      setFiles(data);
    } catch (err) {
      console.error('Failed to load files', err);
    } finally {
      setLoading(false);
    }
  }, [folderId, isSearching, searchQuery]);

  useEffect(() => { loadFiles(); }, [loadFiles]);

  const handleCreateFolder = async () => {
    if (!newFolderName.trim()) return;
    await api.createFolder(newFolderName.trim(), folderId);
    setNewFolderName('');
    setShowNewFolder(false);
    loadFiles();
  };

  const handleRename = async (id: string) => {
    if (!editName.trim()) return;
    await api.renameFile(id, editName.trim());
    setEditingId(null);
    loadFiles();
  };

  const handleDelete = async (id: string) => {
    await api.deleteFile(id);
    loadFiles();
  };

  const handleUpload = async (f: File) => {
    setUploading(prev => [...prev, f.name]);
    try {
      // Compute checksum
      const buf = await f.arrayBuffer();
      const hash = Array.from(new Uint8Array(buf)).map(b => b.toString(16).padStart(2, '0')).join('').slice(0, 64);

      // Initiate
      const { file_id, signed_url } = await api.initUpload(f.name, f.size, f.type || 'application/octet-stream', folderId);

      // Upload to signed URL
      await fetch(signed_url, { method: 'PUT', body: f });

      // Confirm
      await api.confirmUpload(file_id, hash);
      loadFiles();
    } catch (err) {
      console.error('Upload failed', err);
    } finally {
      setUploading(prev => prev.filter(n => n !== f.name));
    }
  };

  const handlePreview = async (f: FileEntry) => {
    const isImage = f.mime_type?.startsWith('image/');
    if (isImage) {
      setPreviewFile(f);
      setPreviewUrl(api.getPreviewUrl(f.id, 800, 600));
    } else {
      window.open(api.getDownloadUrl(f.id), '_blank');
    }
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    const droppedFiles = Array.from(e.dataTransfer.files);
    droppedFiles.forEach(handleUpload);
  };

  const isImage = (f: FileEntry) => f.mime_type?.startsWith('image/');

  return (
    <div>
      {/* Breadcrumb */}
      <div className="flex items-center gap-2 text-sm text-gray-600 mb-4">
        <Link to="/" className="text-gray-500 hover:text-gray-900 no-underline" onClick={() => { setSearchQuery(''); setIsSearching(false); }}>
          <Home className="w-4 h-4" />
        </Link>
        {folderId && <ChevronRight className="w-4 h-4" />}
        {folderId && <span className="text-gray-900 font-medium">Folder</span>}
      </div>

      {/* Toolbar */}
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-2">
          <button
            onClick={() => setShowNewFolder(!showNewFolder)}
            className="flex items-center gap-1 px-3 py-1.5 text-sm bg-white border border-gray-300 rounded-lg hover:bg-gray-50 cursor-pointer"
          >
            <Plus className="w-4 h-4" />
            New Folder
          </button>
          <button
            onClick={() => fileInputRef.current?.click()}
            className="flex items-center gap-1 px-3 py-1.5 text-sm bg-blue-600 text-white rounded-lg hover:bg-blue-700 cursor-pointer border-none"
          >
            Upload
          </button>
          <input ref={fileInputRef} type="file" className="hidden" multiple onChange={e => Array.from(e.target.files || []).forEach(handleUpload)} />
        </div>

        <div className="relative">
          <Search className="absolute left-2.5 top-2.5 w-4 h-4 text-gray-400" />
          <input
            type="text"
            placeholder="Search files..."
            value={searchQuery}
            onChange={e => { setSearchQuery(e.target.value); setIsSearching(!!e.target.value); }}
            className="pl-9 pr-3 py-2 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 w-64"
          />
        </div>
      </div>

      {showNewFolder && (
        <div className="flex gap-2 mb-4">
          <input type="text" value={newFolderName} onChange={e => setNewFolderName(e.target.value)} placeholder="Folder name"
            className="px-3 py-2 border border-gray-300 rounded-lg text-sm flex-1 focus:outline-none focus:ring-2 focus:ring-blue-500"
            onKeyDown={e => e.key === 'Enter' && handleCreateFolder()} />
          <button onClick={handleCreateFolder} className="px-3 py-2 bg-blue-600 text-white rounded-lg text-sm hover:bg-blue-700 cursor-pointer border-none">Create</button>
          <button onClick={() => setShowNewFolder(false)} className="px-3 py-2 border border-gray-300 rounded-lg text-sm hover:bg-gray-50 cursor-pointer bg-white">Cancel</button>
        </div>
      )}

      {/* Upload progress */}
      {uploading.length > 0 && (
        <div className="mb-4">
          {uploading.map(name => (
            <div key={name} className="text-sm text-blue-600 flex items-center gap-2">
              <div className="animate-spin w-3 h-3 border-2 border-blue-600 border-t-transparent rounded-full" />
              Uploading {name}...
            </div>
          ))}
        </div>
      )}

      {/* File list */}
      <div className="bg-white rounded-xl border border-gray-200" onDragOver={e => e.preventDefault()} onDrop={handleDrop}>
        {loading ? (
          <div className="p-8 text-center text-gray-400">Loading...</div>
        ) : files.length === 0 ? (
          <div className="p-12 text-center">
            <p className="text-gray-400 mb-4">This folder is empty</p>
            <p className="text-sm text-gray-400">Drop files here or use the upload button</p>
          </div>
        ) : (
          <div className="divide-y divide-gray-100">
            {files.map(f => (
              <div key={f.id} className="flex items-center gap-3 px-4 py-3 hover:bg-gray-50 group">
                {/* Icon */}
                {f.is_folder ? (
                  <Folder className="w-5 h-5 text-blue-600 flex-shrink-0" />
                ) : isImage(f) ? (
                  <img src={api.getPreviewUrl(f.id, 32, 32)} className="w-5 h-5 rounded object-cover flex-shrink-0" alt="" />
                ) : (
                  <File className="w-5 h-5 text-gray-400 flex-shrink-0" />
                )}

                {/* Name */}
                <div className="flex-1 min-w-0">
                  {editingId === f.id ? (
                    <form onSubmit={e => { e.preventDefault(); handleRename(f.id); }} className="flex gap-2">
                      <input type="text" value={editName} onChange={e => setEditName(e.target.value)}
                        className="px-2 py-1 border border-gray-300 rounded text-sm w-full focus:outline-none focus:ring-2 focus:ring-blue-500"
                        autoFocus onBlur={() => handleRename(f.id)} />
                    </form>
                  ) : (
                    <button
                      onClick={() => f.is_folder ? navigate(`/folder/${f.id}`) : handlePreview(f)}
                      className="text-sm font-medium text-gray-900 hover:text-blue-600 truncate block w-full text-left bg-transparent border-none cursor-pointer"
                    >
                      {f.name}
                    </button>
                  )}
                </div>

                {/* Size */}
                <span className="text-xs text-gray-400 w-20 text-right flex-shrink-0">
                  {f.is_folder ? '' : formatSize(f.size)}
                </span>

                {/* Date */}
                <span className="text-xs text-gray-400 w-24 text-right flex-shrink-0 hidden sm:block">{formatDate(f.created_at)}</span>

                {/* Actions */}
                <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                  {!f.is_folder && (
                    <button onClick={() => { setShareFile(f); }} className="p-1 hover:bg-gray-100 rounded bg-transparent border-none cursor-pointer" title="Share">
                      <Share2 className="w-3.5 h-3.5 text-gray-500" />
                    </button>
                  )}
                  {f.is_folder && (
                    <button onClick={() => { setShareFile(f); }} className="p-1 hover:bg-gray-100 rounded bg-transparent border-none cursor-pointer" title="Share folder">
                      <Share2 className="w-3.5 h-3.5 text-gray-500" />
                    </button>
                  )}
                  <button onClick={() => { setEditingId(f.id); setEditName(f.name); }} className="p-1 hover:bg-gray-100 rounded bg-transparent border-none cursor-pointer" title="Rename">
                    <Pencil className="w-3.5 h-3.5 text-gray-500" />
                  </button>
                  <button onClick={() => handleDelete(f.id)} className="p-1 hover:bg-red-50 rounded bg-transparent border-none cursor-pointer" title="Delete">
                    <Trash2 className="w-3.5 h-3.5 text-red-400" />
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <FileUpload onUpload={handleUpload} />

      {shareFile && <ShareDialog file={shareFile} onClose={() => setShareFile(null)} />}
      {previewFile && <ImagePreview file={previewFile} url={previewUrl} onClose={() => setPreviewFile(null)} />}
    </div>
  );
}

function formatSize(bytes: number): string {
  if (bytes === 0) return '';
  const units = ['B', 'KB', 'MB', 'GB'];
  let i = 0;
  let size = bytes;
  while (size >= 1024 && i < units.length - 1) { size /= 1024; i++; }
  return size.toFixed(i > 0 ? 1 : 0) + ' ' + units[i];
}

function formatDate(dateStr: string): string {
  const d = new Date(dateStr);
  return d.toLocaleDateString('de-DE', { month: 'short', day: 'numeric' });
}
