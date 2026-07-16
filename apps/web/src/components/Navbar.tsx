import React, { useState } from 'react';
import { Link, useNavigate, useLocation } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';
import { BarChart3, CalendarDays, LayoutDashboard, LogOut, MapPin, Menu, Search, Trophy, X, Undo2, Tag, Users, ClipboardList } from 'lucide-react';
import { NotificationDropdown } from './NotificationDropdown';

export const Navbar: React.FC = () => {
  const { user, isAuthenticated, logout, isWorkspaceUser, hasOwnerPermission, isActualOwner } = useAuth();
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
      ? `flex items-center gap-3 rounded-xl px-4 py-3 text-[15px] font-bold transition-all ${isActive ? 'bg-[#05221e] text-white shadow-sm' : 'text-text-muted hover:bg-primary/5 hover:text-text-main'}`
      : `rounded-full px-4 py-2 text-[14px] font-bold transition-all lg:text-[15px] ${isActive ? 'bg-[#05221e] text-white shadow-sm' : 'text-text-muted hover:bg-[#05221e]/5 hover:text-text-main'}`;

    const iconClass = "w-4 h-4 shrink-0";

    return (
      <>
        {isWorkspaceUser() ? (
          <>
            <Link onClick={() => setIsMobileMenuOpen(false)} to="/owner/dashboard" className={linkClass(location.pathname.startsWith('/owner/dashboard'))}>
              {mobile && <LayoutDashboard className={iconClass} />}
              Dashboard
            </Link>
            {hasOwnerPermission('BOOKINGS_READ') && (
              <Link onClick={() => setIsMobileMenuOpen(false)} to="/owner/bookings" className={linkClass(location.pathname.startsWith('/owner/bookings'))}>
                {mobile && <CalendarDays className={iconClass} />}
                Pesanan
              </Link>
            )}
            {hasOwnerPermission('REFUNDS_READ') && (
              <Link onClick={() => setIsMobileMenuOpen(false)} to="/owner/refunds" className={linkClass(location.pathname.startsWith('/owner/refunds'))}>
                {mobile && <Undo2 className={iconClass} />}
                Refunds
              </Link>
            )}
            {hasOwnerPermission('FINANCE_READ') && (
              <Link onClick={() => setIsMobileMenuOpen(false)} to="/owner/finance" className={linkClass(location.pathname.startsWith('/owner/finance'))}>
                {mobile && <BarChart3 className={iconClass} />}
                Keuangan
              </Link>
            )}
            {hasOwnerPermission('VENUES_READ') && (
              <Link onClick={() => setIsMobileMenuOpen(false)} to="/owner/venues" className={linkClass(location.pathname.startsWith('/owner/venues'))}>
                {mobile && <MapPin className={iconClass} />}
                Venue
              </Link>
            )}
            {hasOwnerPermission('PROMOS_READ') && (
              <Link onClick={() => setIsMobileMenuOpen(false)} to="/owner/promos" className={linkClass(location.pathname.startsWith('/owner/promos'))}>
                {mobile && <Tag className={iconClass} />}
                Promo
              </Link>
            )}
            {isActualOwner() && (
              <>
                <Link onClick={() => setIsMobileMenuOpen(false)} to="/owner/staff" className={linkClass(location.pathname.startsWith('/owner/staff'))}>
                  {mobile && <Users className={iconClass} />}
                  Staff
                </Link>
                <Link onClick={() => setIsMobileMenuOpen(false)} to="/owner/audit-logs" className={linkClass(location.pathname.startsWith('/owner/audit-logs'))}>
                  {mobile && <ClipboardList className={iconClass} />}
                  Riwayat Aktivitas
                </Link>
              </>
            )}
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

  const logoHref = isWorkspaceUser() ? '/owner/dashboard' : '/';

  return (
    <nav className={`fixed left-0 top-0 z-50 w-full border-b border-border-main/60 bg-white/92 shadow-[0_16px_45px_-32px_rgba(15,23,42,0.45)] backdrop-blur-xl transition-all md:left-1/2 md:top-5 md:w-[calc(100%-40px)] md:max-w-[1440px] md:-translate-x-1/2 md:border md:border-white/80 ${isMobileMenuOpen ? 'rounded-b-2xl md:rounded-[28px]' : 'md:rounded-[28px]'}`}>
      <div className="flex h-[60px] items-center justify-between px-4 md:h-[68px] md:px-5 lg:px-7">
        <Link to={logoHref} className="flex min-w-0 items-center gap-2.5 text-xl font-extrabold tracking-[-0.04em] md:text-2xl">
          <div className="grid h-9 w-9 shrink-0 place-items-center rounded-xl bg-primary text-lg font-extrabold text-white shadow-[0_10px_24px_-12px_rgba(13,148,136,0.75)] md:h-10 md:w-10 md:text-xl">
            L
          </div>
          <span className="truncate">LapangGo</span>
        </Link>
        <div className="hidden items-center gap-1 rounded-full p-1 md:flex">
          <NavLinks />
        </div>
        <div className="flex shrink-0 items-center gap-1.5 md:gap-2">
          {isAuthenticated && user ? (
            <div className="flex items-center gap-2 md:gap-4">
              <div className="flex items-center gap-2">
                <div className="w-8 h-8 md:w-10 md:h-10 rounded-full bg-primary/10 flex items-center justify-center text-primary font-bold border border-primary/20 text-sm md:text-base">
                  {user.name.charAt(0).toUpperCase()}
                </div>
                <div className="hidden sm:block">
                  <div className="text-[13px] font-bold text-text-main line-clamp-1">{user.name}</div>
                  <div className="text-[11px] font-medium text-text-muted">
                    {user.staff_memberships && user.staff_memberships.length > 0 ? `STAFF - ${user.staff_memberships[0].owner_name}` : user.role}
                  </div>
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
                className="flex h-9 w-9 items-center justify-center rounded-xl text-text-main transition-colors hover:bg-[#05221e]/5 md:hidden"
                aria-label="Buka menu"
                aria-expanded={isMobileMenuOpen}
              >
                {isMobileMenuOpen ? <X className="w-5 h-5" /> : <Menu className="w-5 h-5" />}
              </button>
            </div>
          ) : (
            <>
              <Link to="/login" className="hidden rounded-full px-5 py-2.5 text-[15px] font-bold text-text-main transition-colors hover:bg-[#05221e]/5 md:block">
                Log In
              </Link>
              <Link to="/register" className="rounded-full bg-primary px-4 py-2 text-sm font-bold text-white shadow-[0_10px_24px_-12px_rgba(13,148,136,0.8)] transition-all hover:-translate-y-0.5 hover:bg-[#0b8278] hover:shadow-md md:px-6 md:py-2.5 md:text-[15px]">
                <span className="sm:hidden">Daftar</span>
                <span className="hidden sm:inline">Daftar Akun</span>
              </Link>
              <button
                onClick={() => setIsMobileMenuOpen(!isMobileMenuOpen)}
                className="ml-0.5 flex h-9 w-9 items-center justify-center rounded-xl text-text-main transition-colors hover:bg-[#05221e]/5 md:hidden"
                aria-label="Buka menu"
                aria-expanded={isMobileMenuOpen}
              >
                {isMobileMenuOpen ? <X className="w-5 h-5" /> : <Menu className="w-5 h-5" />}
              </button>
            </>
          )}
        </div>
      </div>

      {/* Mobile Menu */}
      {isMobileMenuOpen && (
        <div className="flex flex-col gap-1 overflow-hidden rounded-b-2xl border-b border-border-main/50 bg-white/95 px-4 py-3 shadow-lg backdrop-blur-xl animate-in slide-in-from-top-2 fade-in duration-200 ease-out md:hidden">
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
