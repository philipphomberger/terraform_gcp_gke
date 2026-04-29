import { useState } from 'react';
import { api } from '../lib/api';
import { useAuth } from '../hooks/useAuth';
import { Shield, Key, QrCode, Trash2, Check, X } from 'lucide-react';

export default function Settings() {
  const { user, refresh } = useAuth();
  const [totpSetup, setTotpSetup] = useState<{ qr_code: string; secret: string; uri: string } | null>(null);
  const [totpCode, setTotpCode] = useState('');
  const [totpError, setTotpError] = useState('');
  const [totpSuccess, setTotpSuccess] = useState('');
  const [loading, setLoading] = useState(false);

  const handleEnableTotp = async () => {
    setLoading(true);
    setTotpError('');
    try {
      const data = await api.totpEnable();
      setTotpSetup(data);
    } catch (err: any) {
      setTotpError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const handleVerifyTotp = async () => {
    setLoading(true);
    setTotpError('');
    try {
      await api.totpVerify(totpCode);
      setTotpSuccess('TOTP enabled successfully');
      setTotpSetup(null);
      setTotpCode('');
      await refresh();
    } catch (err: any) {
      setTotpError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const handleDisableTotp = async () => {
    setLoading(true);
    setTotpError('');
    try {
      await api.totpDisable();
      setTotpSuccess('TOTP disabled');
      await refresh();
    } catch (err: any) {
      setTotpError(err.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="max-w-2xl mx-auto">
      <h2 className="text-xl font-bold text-gray-900 mb-6">Settings</h2>

      {/* Account Info */}
      <div className="bg-white rounded-xl border border-gray-200 p-6 mb-6">
        <div className="flex items-center gap-2 mb-4">
          <Shield className="w-5 h-5 text-gray-700" />
          <h3 className="text-lg font-semibold text-gray-900">Account</h3>
        </div>
        <div className="flex flex-col gap-2 text-sm">
          <div className="flex justify-between py-2 border-b border-gray-100">
            <span className="text-gray-500">Username</span>
            <span className="font-medium text-gray-900">{user?.username}</span>
          </div>
          {user?.email && (
            <div className="flex justify-between py-2 border-b border-gray-100">
              <span className="text-gray-500">Email</span>
              <span className="font-medium text-gray-900">{user.email}</span>
            </div>
          )}
          <div className="flex justify-between py-2">
            <span className="text-gray-500">Member since</span>
            <span className="font-medium text-gray-900">
              {user?.created_at ? new Date(user.created_at).toLocaleDateString() : ''}
            </span>
          </div>
        </div>
      </div>

      {/* TOTP / Two-Factor Auth */}
      <div className="bg-white rounded-xl border border-gray-200 p-6 mb-6">
        <div className="flex items-center gap-2 mb-4">
          <Key className="w-5 h-5 text-gray-700" />
          <h3 className="text-lg font-semibold text-gray-900">Two-Factor Authentication (TOTP)</h3>
        </div>

        {totpSuccess && (
          <div className="mb-4 p-3 bg-green-50 border border-green-200 rounded-lg flex items-center gap-2 text-sm text-green-700">
            <Check className="w-4 h-4" />
            {totpSuccess}
            <button onClick={() => setTotpSuccess('')} className="ml-auto text-green-500 hover:text-green-700 bg-transparent border-none cursor-pointer">
              <X className="w-4 h-4" />
            </button>
          </div>
        )}

        {totpError && (
          <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg text-sm text-red-600">
            {totpError}
          </div>
        )}

        {user?.totp_enabled ? (
          <div>
            <p className="text-sm text-green-700 font-medium mb-3">TOTP is enabled on your account</p>
            <button
              onClick={handleDisableTotp}
              disabled={loading}
              className="flex items-center gap-2 px-4 py-2 bg-red-50 text-red-700 rounded-lg text-sm font-medium hover:bg-red-100 disabled:opacity-50 cursor-pointer border-none"
            >
              <Trash2 className="w-4 h-4" />
              Disable TOTP
            </button>
          </div>
        ) : totpSetup ? (
          <div className="flex flex-col gap-4">
            <div>
              <p className="text-sm text-gray-600 mb-3">
                Scan this QR code with your authenticator app (Google Authenticator, Authy, etc.)
              </p>
              <div className="bg-white border border-gray-200 rounded-lg inline-block p-2">
                <img src={`data:image/png;base64,${totpSetup.qr_code}`} alt="TOTP QR Code" className="w-48 h-48" />
              </div>
              <p className="text-xs text-gray-400 mt-2">
                Manual key: <code className="bg-gray-100 px-1 py-0.5 rounded text-xs">{totpSetup.secret}</code>
              </p>
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Enter verification code</label>
              <div className="flex gap-2">
                <input
                  type="text"
                  value={totpCode}
                  onChange={(e) => setTotpCode(e.target.value)}
                  placeholder="000000"
                  maxLength={6}
                  className="px-3 py-2 border border-gray-300 rounded-lg text-sm w-32 focus:outline-none focus:ring-2 focus:ring-blue-500 tracking-widest text-center"
                  onKeyDown={(e) => e.key === 'Enter' && handleVerifyTotp()}
                />
                <button
                  onClick={handleVerifyTotp}
                  disabled={loading || totpCode.length !== 6}
                  className="px-4 py-2 bg-blue-600 text-white rounded-lg text-sm font-medium hover:bg-blue-700 disabled:opacity-50 cursor-pointer border-none"
                >
                  Verify
                </button>
              </div>
            </div>
          </div>
        ) : (
          <button
            onClick={handleEnableTotp}
            disabled={loading}
            className="flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg text-sm font-medium hover:bg-blue-700 disabled:opacity-50 cursor-pointer border-none"
          >
            <QrCode className="w-4 h-4" />
            Enable TOTP
          </button>
        )}
      </div>

      {/* WebAuthn / Passkeys */}
      <div className="bg-white rounded-xl border border-gray-200 p-6">
        <div className="flex items-center gap-2 mb-4">
          <Key className="w-5 h-5 text-gray-700" />
          <h3 className="text-lg font-semibold text-gray-900">Passkeys (WebAuthn)</h3>
        </div>
        <p className="text-sm text-gray-500">
          Passkey management is available during sign-in. Register new passkeys from the login page.
        </p>
      </div>
    </div>
  );
}
