import React, { useState } from 'react';
import { Outlet, useNavigate, useLocation } from 'react-router-dom';
import { 
  Users, 
  Briefcase, 
  MapPin, 
  Activity, 
  BadgePercent,
  CircleDollarSign,
  LayoutDashboard,
  LogOut,
  Menu,
  X
} from 'lucide-react';
import { useAuth } from '../../contexts/AuthContext';

export const AdminLayout: React.FC = () => {
  const { user, logout } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  const navItems = [
    { name: 'Dashboard', path: '/admin/dashboard', icon: LayoutDashboard },
    { name: 'Users', path: '/admin/users', icon: Users },
    { name: 'Owners', path: '/admin/owners', icon: Briefcase },
    { name: 'Venues', path: '/admin/venues', icon: MapPin },
    { name: 'Commercial Terms', path: '/admin/commercial-terms', icon: BadgePercent },
    { name: 'Platform Finance', path: '/admin/finance', icon: CircleDollarSign },
    { name: 'Audit Logs', path: '/admin/audit-logs', icon: Activity },
  ];

  return (
    <div className="min-h-screen bg-slate-50 flex">
      {/* Desktop Sidebar */}
      <aside className="hidden md:flex flex-col w-64 bg-slate-900 text-slate-300 transition-all duration-300">
        <div className="h-16 flex items-center px-6 border-b border-slate-800">
          <span className="text-xl font-bold text-white tracking-tight">LapangGo <span className="text-emerald-500">Admin</span></span>
        </div>
        <div className="flex-1 overflow-y-auto py-4">
          <nav className="space-y-1 px-3">
            {navItems.map((item) => {
              const isActive = location.pathname.startsWith(item.path);
              const Icon = item.icon;
              return (
                <button
                  key={item.path}
                  onClick={() => navigate(item.path)}
                  className={`w-full flex items-center px-3 py-2.5 text-sm font-medium rounded-lg transition-colors ${
                    isActive
                      ? 'bg-emerald-500/10 text-emerald-400'
                      : 'hover:bg-slate-800 hover:text-white'
                  }`}
                >
                  <Icon className={`mr-3 h-5 w-5 ${isActive ? 'text-emerald-400' : 'text-slate-500'}`} />
                  {item.name}
                </button>
              );
            })}
          </nav>
        </div>
        <div className="p-4 border-t border-slate-800">
          <div className="flex items-center mb-4 px-2">
            <div className="h-8 w-8 rounded-full bg-emerald-500 flex items-center justify-center text-white font-bold text-sm">
              {user?.name?.charAt(0).toUpperCase()}
            </div>
            <div className="ml-3 truncate">
              <p className="text-sm font-medium text-white truncate">{user?.name}</p>
              <p className="text-xs text-slate-400 truncate">{user?.email}</p>
            </div>
          </div>
          <button
            onClick={handleLogout}
            className="w-full flex items-center justify-center px-4 py-2 border border-slate-700 rounded-lg text-sm font-medium hover:bg-slate-800 transition-colors"
          >
            <LogOut className="mr-2 h-4 w-4" />
            Logout
          </button>
        </div>
      </aside>

      {/* Mobile Menu */}
      <div className="md:hidden flex flex-col w-full h-screen fixed inset-0 z-50 pointer-events-none">
        <div className="h-16 bg-slate-900 flex items-center justify-between px-4 pointer-events-auto">
          <span className="text-lg font-bold text-white">LapangGo Admin</span>
          <button
            onClick={() => setIsMobileMenuOpen(!isMobileMenuOpen)}
            className="text-slate-300 hover:text-white p-2"
          >
            {isMobileMenuOpen ? <X className="h-6 w-6" /> : <Menu className="h-6 w-6" />}
          </button>
        </div>
        
        {isMobileMenuOpen && (
          <div className="flex-1 bg-slate-900 pointer-events-auto overflow-y-auto">
            <nav className="px-4 py-6 space-y-2">
              {navItems.map((item) => {
                const isActive = location.pathname.startsWith(item.path);
                const Icon = item.icon;
                return (
                  <button
                    key={item.path}
                    onClick={() => {
                      navigate(item.path);
                      setIsMobileMenuOpen(false);
                    }}
                    className={`w-full flex items-center px-4 py-3 text-base font-medium rounded-xl transition-colors ${
                      isActive
                        ? 'bg-emerald-500/10 text-emerald-400'
                        : 'text-slate-300 hover:bg-slate-800 hover:text-white'
                    }`}
                  >
                    <Icon className={`mr-4 h-5 w-5 ${isActive ? 'text-emerald-400' : 'text-slate-500'}`} />
                    {item.name}
                  </button>
                );
              })}
              <div className="pt-6 mt-6 border-t border-slate-800">
                <button
                  onClick={handleLogout}
                  className="w-full flex items-center px-4 py-3 text-base font-medium text-slate-300 rounded-xl hover:bg-slate-800 transition-colors"
                >
                  <LogOut className="mr-4 h-5 w-5 text-slate-500" />
                  Logout
                </button>
              </div>
            </nav>
          </div>
        )}
      </div>

      {/* Main Content */}
      <main className="flex-1 flex flex-col min-w-0 overflow-hidden pt-16 md:pt-0">
        <div className="flex-1 overflow-y-auto p-4 md:p-8">
          <Outlet />
        </div>
      </main>
    </div>
  );
};
