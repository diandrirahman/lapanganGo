import React, { useState } from 'react';
import { Link, useNavigate, useLocation } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';
import { LogOut, Menu, X } from 'lucide-react';

export const Navbar: React.FC = () => {
  const { user, isAuthenticated, logout } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);

  const handleLogout = () => {
    logout();
    navigate('/');
    setIsMobileMenuOpen(false);
  };

  const NavLinks = () => (
    <>
      {user?.role === 'OWNER' ? (
        <>
          <Link onClick={() => setIsMobileMenuOpen(false)} to="/owner/dashboard" className={`text-[15px] font-semibold transition-colors ${location.pathname.startsWith('/owner/dashboard') ? 'text-text-main' : 'text-text-muted hover:text-text-main'}`}>Dashboard</Link>
          <Link onClick={() => setIsMobileMenuOpen(false)} to="/owner/venues" className={`text-[15px] font-semibold transition-colors ${location.pathname.startsWith('/owner/venues') ? 'text-text-main' : 'text-text-muted hover:text-text-main'}`}>Kelola Venue</Link>
        </>
      ) : (
        <>
          <Link onClick={() => setIsMobileMenuOpen(false)} to="/venues" className={`text-[15px] font-semibold transition-colors ${location.pathname.startsWith('/venues') ? 'text-text-main' : 'text-text-muted hover:text-text-main'}`}>Temukan Venue</Link>
          <Link onClick={() => setIsMobileMenuOpen(false)} to="/open-matches" className={`text-[15px] font-semibold transition-colors ${location.pathname.startsWith('/open-matches') ? 'text-text-main' : 'text-text-muted hover:text-text-main'}`}>Mabar (Open Match)</Link>
          {isAuthenticated && (
            <Link onClick={() => setIsMobileMenuOpen(false)} to="/bookings" className={`text-[15px] font-semibold transition-colors ${location.pathname.startsWith('/bookings') ? 'text-text-main' : 'text-text-muted hover:text-text-main'}`}>Pesanan Saya</Link>
          )}
        </>
      )}
    </>
  );

  return (
    <nav className="fixed top-6 left-1/2 -translate-x-1/2 w-[calc(100%-48px)] max-w-6xl z-50 bg-white/85 backdrop-blur-xl border border-white/80 rounded-full shadow-sm">
      <div className="h-[76px] flex justify-between items-center px-6">
        <Link to="/" className="flex items-center gap-2 text-2xl font-extrabold tracking-tight">
          <div className="w-10 h-10 bg-primary-gradient rounded-xl grid place-items-center text-white shadow-primary-glow font-bold text-xl">
            L
          </div>
          LapanganGo
        </Link>
        <div className="hidden md:flex gap-8 items-center">
          <NavLinks />
        </div>
        <div className="flex gap-3 items-center">
          {isAuthenticated && user ? (
            <div className="flex items-center gap-4">
              <div className="flex items-center gap-2">
                <div className="w-10 h-10 rounded-full bg-primary/10 flex items-center justify-center text-primary font-bold border border-primary/20">
                  {user.name.charAt(0).toUpperCase()}
                </div>
                <div className="hidden sm:block">
                  <div className="text-[13px] font-bold text-text-main line-clamp-1">{user.name}</div>
                  <div className="text-[11px] font-medium text-text-muted">{user.role}</div>
                </div>
              </div>
              <button 
                onClick={handleLogout}
                className="w-10 h-10 rounded-full flex items-center justify-center text-text-muted hover:bg-red-50 hover:text-red-500 transition-colors"
                title="Logout"
              >
                <LogOut className="w-5 h-5" />
              </button>
              <button 
                onClick={() => setIsMobileMenuOpen(!isMobileMenuOpen)}
                className="md:hidden w-10 h-10 rounded-full flex items-center justify-center text-text-main hover:bg-gray-100 transition-colors"
              >
                {isMobileMenuOpen ? <X className="w-5 h-5" /> : <Menu className="w-5 h-5" />}
              </button>
            </div>
          ) : (
            <>
              <Link to="/login" className="hidden md:block px-6 py-2.5 rounded-full font-bold text-[15px] text-text-main hover:bg-black/5 transition-colors">
                Log In
              </Link>
              <Link to="/register" className="px-6 py-2.5 rounded-full font-bold text-[15px] transition-all bg-primary-gradient text-white shadow-primary-glow hover:-translate-y-1">
                Daftar Akun
              </Link>
              <button 
                onClick={() => setIsMobileMenuOpen(!isMobileMenuOpen)}
                className="md:hidden w-10 h-10 ml-2 rounded-full flex items-center justify-center text-text-main hover:bg-gray-100 transition-colors"
              >
                {isMobileMenuOpen ? <X className="w-5 h-5" /> : <Menu className="w-5 h-5" />}
              </button>
            </>
          )}
        </div>
      </div>
      
      {/* Mobile Menu */}
      {isMobileMenuOpen && (
        <div className="md:hidden border-t border-border-main bg-white/95 backdrop-blur-xl rounded-b-3xl overflow-hidden px-6 py-4 flex flex-col gap-4 shadow-lg animate-in slide-in-from-top-4">
          <NavLinks />
          {!isAuthenticated && (
            <Link onClick={() => setIsMobileMenuOpen(false)} to="/login" className="text-[15px] font-semibold text-text-main hover:text-text-main transition-colors">Log In</Link>
          )}
        </div>
      )}
    </nav>
  );
};
