const BASE = '/api';

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...options.headers },
    ...options,
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || 'Request failed');
  }
  return res.json();
}

export interface User {
  id: string;
  username: string;
  email?: string;
  display_name?: string;
  totp_enabled: boolean;
  created_at: string;
}

export interface Session {
  token: string;
  user: User;
  expires: string;
}

export interface FileEntry {
  id: string;
  user_id: string;
  parent_id?: string;
  name: string;
  size: number;
  mime_type?: string;
  is_folder: boolean;
  status: string;
  created_at: string;
  updated_at: string;
}

export interface Share {
  id: string;
  owner_id: string;
  file_id: string;
  share_type: 'user' | 'anonymous';
  token?: string;
  permissions: { read: boolean; write: boolean };
  expires_at?: string;
  created_at: string;
}

// Auth
export const api = {
  register: (username: string, password: string) =>
    request<User>('/auth/register', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),

  login: (username: string, password: string, totp_code?: string) =>
    request<Session>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password, totp_code }),
    }),

  logout: () =>
    request<{ status: string }>('/auth/logout', { method: 'POST' }),

  me: () => request<User>('/auth/me'),

  totpEnable: () =>
    request<{ secret: string; uri: string; qr_code: string }>('/auth/totp/enable', { method: 'POST' }),

  totpVerify: (code: string) =>
    request<{ status: string }>('/auth/totp/verify', {
      method: 'POST',
      body: JSON.stringify({ code }),
    }),

  totpDisable: () =>
    request<{ status: string }>('/auth/totp/disable', { method: 'POST' }),

  // Files
  initUpload: (name: string, size: number, mimeType: string, parentId?: string) =>
    request<{ file_id: string; signed_url: string; file: FileEntry }>('/files/upload', {
      method: 'POST',
      body: JSON.stringify({ name, size, mime_type: mimeType, parent_id: parentId }),
    }),

  confirmUpload: (file_id: string, checksum: string) =>
    request<FileEntry>('/files/upload/complete', {
      method: 'POST',
      body: JSON.stringify({ file_id, checksum }),
    }),

  listFiles: (parentId?: string) => {
    const q = parentId ? `?parent_id=${parentId}` : '';
    return request<FileEntry[]>(`/files${q}`);
  },

  createFolder: (name: string, parentId?: string) =>
    request<FileEntry>('/files/folder', {
      method: 'POST',
      body: JSON.stringify({ name, parent_id: parentId }),
    }),

  renameFile: (id: string, name: string) =>
    request<FileEntry>(`/files/${id}/rename`, {
      method: 'PATCH',
      body: JSON.stringify({ name }),
    }),

  moveFile: (id: string, parentId: string) =>
    request<FileEntry>(`/files/${id}/move`, {
      method: 'PATCH',
      body: JSON.stringify({ parent_id: parentId }),
    }),

  deleteFile: (id: string) =>
    request<{ status: string }>(`/files/${id}`, { method: 'DELETE' }),

  getDownloadUrl: (id: string) => `${BASE}/files/${id}/download`,

  getPreviewUrl: (id: string, w = 400, h = 300) => `${BASE}/files/${id}/preview?w=${w}&h=${h}`,

  // Shares
  createShare: (fileId: string, shareType: 'user' | 'anonymous', permissions: { read: boolean; write: boolean }, recipientId?: string) =>
    request<Share>('/shares', {
      method: 'POST',
      body: JSON.stringify({ file_id: fileId, share_type: shareType, permissions, recipient_id: recipientId }),
    }),

  listShares: () => request<Share[]>('/shares'),

  getShare: (token: string) =>
    request<{ share: Share; files: FileEntry[] }>(`/shares/${token}`),

  anonInitUpload: (token: string, name: string, size: number, mimeType: string) =>
    request<{ file_id: string; signed_url: string }>(`/shares/${token}/upload`, {
      method: 'POST',
      body: JSON.stringify({ name, size, mime_type: mimeType }),
    }),

  anonConfirmUpload: (token: string, file_id: string, checksum: string) =>
    request<FileEntry>(`/shares/${token}/upload/complete`, {
      method: 'POST',
      body: JSON.stringify({ file_id, checksum }),
    }),

  anonDownloadUrl: (token: string, fileId: string) => `${BASE}/shares/${token}/files/${fileId}/download`,
};
