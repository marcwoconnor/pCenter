import { useState, useRef } from 'react';
import { getCSRFToken } from '../api/auth';

interface UploadDialogProps {
  storage: string;
  node: string;
  contentType: 'iso' | 'vztmpl';
  onClose: () => void;
}

export function UploadDialog({ storage, node, contentType, onClose }: UploadDialogProps) {
  const [file, setFile] = useState<File | null>(null);
  const [uploading, setUploading] = useState(false);
  const [progress, setProgress] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const title = contentType === 'iso' ? 'Upload ISO' : 'Upload Template';
  const accept = contentType === 'iso' ? '.iso' : '.tar.gz,.tar.xz,.tar.zst';

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = e.target.files;
    if (files && files.length > 0) {
      setFile(files[0]);
      setError(null);
    }
  };

  const handleUpload = async () => {
    if (!file) return;

    setUploading(true);
    setError(null);
    setProgress(0);

    try {
      const formData = new FormData();
      formData.append('file', file);

      const xhr = new XMLHttpRequest();

      xhr.upload.addEventListener('progress', (e) => {
        if (e.lengthComputable) {
          setProgress(Math.round((e.loaded / e.total) * 100));
        }
      });

      xhr.addEventListener('load', () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          onClose();
        } else {
          try {
            const resp = JSON.parse(xhr.responseText);
            setError(resp.error || 'Upload failed');
          } catch {
            setError('Upload failed: ' + xhr.statusText);
          }
        }
        setUploading(false);
      });

      xhr.addEventListener('error', () => {
        setError('Upload failed: network error');
        setUploading(false);
      });

      xhr.open('POST', `/api/storage/${storage}/upload?node=${node}&content=${contentType}`);
      const csrfToken = getCSRFToken();
      if (csrfToken) {
        xhr.setRequestHeader('X-CSRF-Token', csrfToken);
      }
      xhr.withCredentials = true;
      xhr.send(formData);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Upload failed');
      setUploading(false);
    }
  };

  const formatSize = (bytes: number) => {
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
    if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
    return (bytes / (1024 * 1024 * 1024)).toFixed(2) + ' GB';
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={onClose}>
      <div
        className="bg-white dark:bg-gray-800 rounded-lg shadow-xl p-6 w-full max-w-md"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">{title}</h2>

        <div className="mb-4">
          <div className="text-sm text-gray-500 dark:text-gray-400 mb-2">
            Storage: <span className="font-medium text-gray-700 dark:text-gray-300">{storage}</span> on <span className="font-medium text-gray-700 dark:text-gray-300">{node}</span>
          </div>
        </div>

        <div className="mb-4">
          <input
            ref={fileInputRef}
            type="file"
            accept={accept}
            onChange={handleFileChange}
            className="hidden"
          />
          <button
            onClick={() => fileInputRef.current?.click()}
            disabled={uploading}
            className="w-full px-4 py-8 border-2 border-dashed border-gray-300 dark:border-gray-600 rounded-lg
              hover:border-gray-400 dark:hover:border-gray-500 transition-colors
              text-gray-500 dark:text-gray-400 text-center"
          >
            {file ? (
              <div>
                <div className="font-medium text-gray-700 dark:text-gray-300">{file.name}</div>
                <div className="text-sm">{formatSize(file.size)}</div>
              </div>
            ) : (
              <div>Click to select {contentType === 'iso' ? 'an ISO file' : 'a template'}</div>
            )}
          </button>
        </div>

        {uploading && (
          <div className="mb-4">
            <div className="flex justify-between text-sm text-gray-500 dark:text-gray-400 mb-1">
              <span>Uploading...</span>
              <span>{progress}%</span>
            </div>
            <div className="h-2 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
              <div
                className="h-full bg-blue-500 transition-all duration-300"
                style={{ width: `${progress}%` }}
              />
            </div>
          </div>
        )}

        {error && (
          <div className="mb-4 p-3 bg-red-50 dark:bg-red-900/20 text-red-600 dark:text-red-400 rounded text-sm">
            {error}
          </div>
        )}

        <div className="flex justify-end gap-3">
          <button
            onClick={onClose}
            disabled={uploading}
            className="px-4 py-2 text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded transition-colors"
          >
            Cancel
          </button>
          <button
            onClick={handleUpload}
            disabled={!file || uploading}
            className="px-4 py-2 bg-blue-600 hover:bg-blue-700 disabled:bg-gray-400 text-white rounded transition-colors"
          >
            {uploading ? 'Uploading...' : 'Upload'}
          </button>
        </div>
      </div>
    </div>
  );
}
