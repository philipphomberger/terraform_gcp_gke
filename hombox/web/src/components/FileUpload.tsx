import { useState, useRef, useCallback } from 'react';
import { Upload } from 'lucide-react';

interface FileUploadProps {
  onUpload: (file: File) => void;
}

export default function FileUpload({ onUpload }: FileUploadProps) {
  const [isDragOver, setIsDragOver] = useState(false);
  const dragCounter = useRef(0);

  const handleDragEnter = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    dragCounter.current++;
    setIsDragOver(true);
  }, []);

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    dragCounter.current--;
    if (dragCounter.current === 0) {
      setIsDragOver(false);
    }
  }, []);

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    dragCounter.current = 0;
    setIsDragOver(false);
    Array.from(e.dataTransfer.files).forEach(onUpload);
  }, [onUpload]);

  return (
    <div
      onDragEnter={handleDragEnter}
      onDragLeave={handleDragLeave}
      onDragOver={(e) => e.preventDefault()}
      onDrop={handleDrop}
      className={`fixed bottom-6 left-1/2 -translate-x-1/2 transition-all duration-200 ${
        isDragOver ? 'scale-110' : ''
      }`}
    >
      <div
        className={`px-6 py-4 rounded-2xl border-2 border-dashed transition-colors ${
          isDragOver
            ? 'border-blue-400 bg-blue-50 shadow-lg'
            : 'border-gray-300 bg-white/80 backdrop-blur shadow-sm'
        }`}
      >
        <div className="flex items-center gap-3 text-sm text-gray-500">
          <Upload className={`w-5 h-5 ${isDragOver ? 'text-blue-500' : 'text-gray-400'}`} />
          <span>Drop files here to upload</span>
        </div>
      </div>
    </div>
  );
}
