import { Outlet, Link, useNavigate } from 'react-router-dom';
import { useAuth } from '../hooks/useAuth';
import { api } from '../lib/api';
import { Folder, Settings, LogOut } from 'lucide-react';

export default function Layout() {
  const { user, refresh } = useAuth();
  const navigate = useNavigate();

  const handleLogout = async () => {
    await api.logout();
    await refresh();
    navigate('/login');
  };

  return (
    <div className="min-h-screen bg-gray-50">
      <nav className="bg-white border-b border-gray-200 px-6 py-3 flex items-center justify-between">
        <div className="flex items-center gap-6">
          <Link to="/" className="flex items-center gap-2 font-bold text-xl text-blue-600 no-underline">
            <Folder className="w-6 h-6" />
            Hombox
          </Link>
        </div>
        <div className="flex items-center gap-4">
          <Link to="/settings" className="flex items-center gap-1 text-sm text-gray-600 hover:text-gray-900 no-underline">
            <Settings className="w-4 h-4" />
            Settings
          </Link>
          <span className="text-sm text-gray-500">{user?.username}</span>
          <button onClick={handleLogout} className="flex items-center gap-1 text-sm text-gray-600 hover:text-red-600 bg-transparent border-none cursor-pointer">
            <LogOut className="w-4 h-4" />
          </button>
        </div>
      </nav>
      <main className="max-w-5xl mx-auto p-6">
        <Outlet />
      </main>
    </div>
  );
}
