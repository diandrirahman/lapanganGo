import React, { useState } from 'react';
import { Link, useNavigate, useLocation } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';
import { BarChart3, CalendarDays, LayoutDashboard, LogOut, MapPin, Menu, Search, Trophy, X, Undo2 } from 'lucide-react';
import { NotificationDropdown } from './NotificationDropdown';

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

  const NavLinks = ({ mobile = false }: { mobile?: boolean }) => {
    const linkClass = (isActive: boolean) => mobile
      ? `flex items-center gap-3 rounded-xl px-4 py-3 text-[15px] font-bold transition-colors ${isActive ? 'bg-primary/10 text-primary' : 'text-text-muted hover:bg-gray-50 hover:text-text-main'}`
      : `text-[14px] lg:text-[15px] font-semibold transition-colors ${isActive ? 'text-text-main' : 'text-text-muted hover:text-text-main'}`;

    const iconClass = "w-4 h-4 shrink-0";

    return (
      <>
      {user?.role === 'OWNER' ? (
        <>
          <Link onClick={() => setIsMobileMenuOpen(false)} to="/owner/dashboard" className={linkClass(location.pathname.startsWith('/owner/dashboard'))}>
            {mobile && <LayoutDashboard className={iconClass} />}
            Dashboard
          </Link>
          <Link onClick={() => setIsMobileMenuOpen(false)} to="/owner/bookings" className={linkClass(location.pathname.startsWith('/owner/bookings'))}>
            {mobile && <CalendarDays className={iconClass} />}
            Pesanan
          </Link>
          <Link onClick={() => setIsMobileMenuOpen(false)} to="/owner/refunds" className={linkClass(location.pathname.startsWith('/owner/refunds'))}>
            {mobile && <Undo2 className={iconClass} />}
            Refunds
          </Link>
          <Link onClick={() => setIsMobileMenuOpen(false)} to="/owner/finance" className={linkClass(location.pathname.startsWith('/owner/finance'))}>
            {mobile && <BarChart3 className={iconClass} />}
            Keuangan
          </Link>
          <Link onClick={() => setIsMobileMenuOpen(false)} to="/owner/venues" className={linkClass(location.pathname.startsWith('/owner/venues'))}>
            {mobile && <MapPin className={iconClass} />}
            Venue
          </Link>
        </>
      ) : (
        <>
          <Link onClick={() => setIsMobileMenuOpen(false)} to="/venues" className={linkClass(location.pathname.startsWith('/venues'))}>
            {mobile && <Search className={iconClass} />}
            Temukan Venue
          </Link>
          <Link onClick={() => setIsMobileMenuOpen(false)} to="/open-matches" className={linkClass(location.pathname.startsWith('/open-matches'))}>
            {mobile && <Trophy className={iconClass} />}
            Mabar
          </Link>
          {isAuthenticated && (
            <Link onClick={() => setIsMobileMenuOpen(false)} to="/bookings" className={linkClass(location.pathname.startsWith('/bookings'))}>
              {mobile && <CalendarDays className={iconClass} />}
              Pesanan Saya
            </Link>
          )}
        </>
      )}
      </>
    );
  };

  const logoHref = user?.role === 'OWNER' ? '/owner/dashboard' : '/';

  return (
    <nav className={`fixed top-0 md:top-6 left-0 md:left-1/2 md:-translate-x-1/2 w-full md:w-[calc(100%-40px)] max-w-6xl z-50 bg-white/95 backdrop-blur-xl border-b md:border border-border-main/50 md:border-white/80 shadow-sm transition-all ${isMobileMenuOpen ? 'rounded-b-2xl md:rounded-[28px]' : 'md:rounded-full'}`}>
      <div className="h-14 md:h-[72px] flex justify-between items-center px-4 md:px-6">
        <Link to={logoHref} className="flex items-center gap-2 min-w-0 text-lg md:text-2xl font-extrabold tracking-tight">
          <div className="w-8 h-8 md:w-11 md:h-11 bg-primary rounded-lg md:rounded-xl grid place-items-center text-white shadow-sm font-bold text-lg md:text-xl shrink-0">
            L
          </div>
          <span className="truncate">LapanganGo</span>
        </Link>
        <div className="hidden md:flex gap-5 lg:gap-8 items-center">
          <NavLinks />
        </div>
        <div className="flex gap-2 md:gap-3 items-center shrink-0">
          {isAuthenticated && user ? (
            <div className="flex items-center gap-2 md:gap-4">
              <div className="flex items-center gap-2">
                <div className="w-8 h-8 md:w-10 md:h-10 rounded-full bg-primary/10 flex items-center justify-center text-primary font-bold border border-primary/20 text-sm md:text-base">
                  {user.name.charAt(0).toUpperCase()}
                </div>
                <div className="hidden sm:block">
                  <div className="text-[13px] font-bold text-text-main line-clamp-1">{user.name}</div>
                  <div className="text-[11px] font-medium text-text-muted">{user.role}</div>
                </div>
              </div>
              <NotificationDropdown />
              <button 
                onClick={handleLogout}
                className="hidden md:flex w-10 h-10 rounded-full items-center justify-center text-text-muted hover:bg-red-50 hover:text-red-500 transition-colors"
                title="Logout"
              >
                <LogOut className="w-5 h-5" />
              </button>
              <button 
                onClick={() => setIsMobileMenuOpen(!isMobileMenuOpen)}
                className="md:hidden w-9 h-9 rounded-full flex items-center justify-center text-text-main hover:bg-gray-100 transition-colors"
                aria-label="Buka menu"
              >
                {isMobileMenuOpen ? <X className="w-5 h-5" /> : <Menu className="w-5 h-5" />}
              </button>
            </div>
          ) : (
            <>
              <Link to="/login" className="hidden md:block px-6 py-2.5 rounded-full font-bold text-[15px] text-text-main hover:bg-black/5 transition-colors">
                Log In
              </Link>
              <Link to="/register" className="px-5 md:px-6 py-2 md:py-2.5 rounded-full font-bold text-sm md:text-[15px] transition-all bg-primary text-white shadow-sm hover:shadow-md hover:-translate-y-1">
                Daftar Akun
              </Link>
              <button 
                onClick={() => setIsMobileMenuOpen(!isMobileMenuOpen)}
                className="md:hidden w-8 h-8 ml-1 rounded-full flex items-center justify-center text-text-main hover:bg-gray-100 transition-colors"
                aria-label="Buka menu"
              >
                {isMobileMenuOpen ? <X className="w-5 h-5" /> : <Menu className="w-5 h-5" />}
              </button>
            </>
          )}
        </div>
      </div>
      
      {/* Mobile Menu */}
      {isMobileMenuOpen && (
        <div className="md:hidden bg-white/95 backdrop-blur-xl rounded-b-2xl overflow-hidden px-4 py-3 flex flex-col gap-1 shadow-lg border-b border-border-main/50 animate-in slide-in-from-top-2 fade-in duration-200 ease-out">
          <NavLinks mobile />
          {!isAuthenticated && (
            <Link onClick={() => setIsMobileMenuOpen(false)} to="/login" className="flex items-center gap-3 rounded-xl px-4 py-3 text-[15px] font-bold text-text-muted hover:bg-gray-50 hover:text-text-main transition-colors">Log In</Link>
          )}
          {isAuthenticated && (
            <button
              onClick={handleLogout}
              className="flex items-center gap-3 rounded-xl px-4 py-3 text-[15px] font-bold text-red-500 hover:bg-red-50 transition-colors text-left"
            >
              <LogOut className="w-4 h-4" />
              Keluar
            </button>
          )}
        </div>
      )}
    </nav>
  );
};
