import React, { useEffect, useState, useCallback } from 'react';
import { PageShell } from '../../components/layout/PageShell';
import { useAuth } from '../../contexts/AuthContext';
import { useNavigate, Link } from 'react-router-dom';
import { fetchOwnerProfile, fetchOwnerMetrics, fetchAnalyticsBookingsTrend, fetchAnalyticsRevenueTrend } from '../../lib/api';
import type { OwnerProfile, OwnerMetrics } from '../../types/owner';
import type { BookingTrendItem } from '../../types/analytics';
import { Building2, CalendarDays, Wallet, User, ArrowRight, AlertCircle } from 'lucide-react';
import { BookingsChart } from '../../components/owner/BookingsChart';
import { RevenueChart } from '../../components/owner/RevenueChart';

const getMonthDateRange = () => {
  const now = new Date();
  const y = now.getFullYear();
  const m = String(now.getMonth() + 1).padStart(2, '0');
  const d = new Date(y, now.getMonth() + 1, 0).getDate();
  return { startDate: `${y}-${m}-01`, endDate: `${y}-${m}-${d}` };
};

export const OwnerDashboardPage: React.FC = () => {
  const { token } = useAuth();
  const [profile, setProfile] = useState<OwnerProfile | null>(null);
  const [metrics, setMetrics] = useState<OwnerMetrics | null>(null);
  const [bookingsTrend, setBookingsTrend] = useState<BookingTrendItem[]>([]);
  const [revenueBreakdown, setRevenueBreakdown] = useState<{name: string, value: number}[]>([]);
  const [trendError, setTrendError] = useState(false);
  const [revenueError, setRevenueError] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const monthRange = getMonthDateRange();
  const [startDate, setStartDate] = useState(monthRange.startDate);
  const [endDate, setEndDate] = useState(monthRange.endDate);
  const navigate = useNavigate();

  const loadDashboard = useCallback(() => {
    if (token) {
      setIsLoading(true);
      Promise.all([
        fetchOwnerProfile(token).then(setProfile).catch(console.error),
        fetchOwnerMetrics(token, startDate, endDate).then(setMetrics).catch(console.error),
        fetchAnalyticsBookingsTrend(token, { start_date: startDate, end_date: endDate })
          .then(res => {
            setBookingsTrend(res.trend || []);
            setTrendError(false);
          })
          .catch(e => {
            console.error(e);
            setTrendError(true);
          }),
        fetchAnalyticsRevenueTrend(token, { start_date: startDate, end_date: endDate })
          .then(res => {
             const bd = res.venue_breakdown || [];
             setRevenueBreakdown(bd.map(i => ({ name: i.venue_name, value: i.revenue })));
             setRevenueError(false);
          })
          .catch(e => {
            console.error(e);
            setRevenueError(true);
          })
      ]).finally(() => setIsLoading(false));
    } else {
      setIsLoading(false);
    }
  }, [token, startDate, endDate]);

  useEffect(() => {
    loadDashboard();
  }, [loadDashboard]);

  if (isLoading) {
    return <PageShell><div className="pt-32 text-center text-text-muted">Memuat Dashboard...</div></PageShell>;
  }

  return (
    <PageShell>
      <div className="pt-24 pb-40 max-w-6xl mx-auto px-6">
        <div className="flex flex-col md:flex-row justify-between items-start md:items-end gap-6 mb-8">
          <div>
            <h1 className="text-2xl md:text-3xl lg:text-4xl font-extrabold text-text-main mb-1 md:mb-2">Dashboard Owner</h1>
            <p className="text-text-muted text-sm md:text-lg">Selamat datang kembali, kelola bisnis lapangan Anda di sini.</p>
          </div>
          
          <div className="flex flex-col sm:flex-row gap-3 sm:gap-4 w-full md:w-auto">
            <div className="flex-1 sm:flex-none">
              <label className="block text-xs font-bold text-text-muted mb-1">Mulai Tanggal</label>
              <input 
                type="date"
                value={startDate}
                onChange={(e) => setStartDate(e.target.value)}
                className="w-full px-3 py-2 rounded-xl border border-border-main text-sm outline-none focus:border-primary focus:ring-1 focus:ring-primary bg-white"
              />
            </div>
            <div className="flex-1 sm:flex-none">
              <label className="block text-xs font-bold text-text-muted mb-1">Sampai Tanggal</label>
              <input 
                type="date"
                value={endDate}
                onChange={(e) => setEndDate(e.target.value)}
                className="w-full px-3 py-2 rounded-xl border border-border-main text-sm outline-none focus:border-primary focus:ring-1 focus:ring-primary bg-white"
              />
            </div>
          </div>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 md:gap-4 mb-4">
          {/* Stats Cards */}
          <div className="bg-white p-4 md:p-5 rounded-2xl border border-border-main shadow-sm flex items-center gap-3 md:gap-4">
            <div className="w-10 h-10 md:w-12 md:h-12 bg-blue-50 text-blue-600 rounded-xl flex items-center justify-center shrink-0">
              <Building2 className="w-5 h-5 md:w-6 md:h-6" />
            </div>
            <div className="min-w-0">
              <p className="text-text-muted font-bold text-[11px] md:text-xs truncate">Total Venue</p>
              <p className="text-lg md:text-xl font-extrabold text-text-main truncate">{metrics ? metrics.total_venues : '...'}</p>
            </div>
          </div>
          
          <button 
            onClick={() => navigate('/owner/bookings?tab=mendatang')}
            className="bg-white p-4 md:p-5 rounded-2xl border border-border-main shadow-sm flex items-center gap-3 md:gap-4 hover:border-primary hover:shadow-md transition-all text-left w-full"
          >
            <div className="w-10 h-10 md:w-12 md:h-12 bg-green-50 text-green-600 rounded-xl flex items-center justify-center shrink-0">
              <CalendarDays className="w-5 h-5 md:w-6 md:h-6" />
            </div>
            <div className="min-w-0">
              <p className="text-text-muted font-bold text-[11px] md:text-xs truncate">Pesanan Mendatang</p>
              <p className="text-lg md:text-xl font-extrabold text-text-main truncate">{metrics ? metrics.upcoming_bookings : '...'}</p>
            </div>
          </button>

          <button 
            onClick={() => navigate('/owner/bookings?status=WAITING_VERIFICATION')}
            className="bg-white p-4 md:p-5 rounded-2xl border border-border-main shadow-sm flex items-center gap-3 md:gap-4 hover:border-yellow-500 hover:shadow-md transition-all text-left w-full"
          >
            <div className="w-10 h-10 md:w-12 md:h-12 bg-yellow-50 text-yellow-600 rounded-xl flex items-center justify-center shrink-0">
              <AlertCircle className="w-5 h-5 md:w-6 md:h-6" />
            </div>
            <div className="min-w-0">
              <p className="text-text-muted font-bold text-[11px] md:text-xs truncate">Menunggu Verifikasi</p>
              <p className="text-lg md:text-xl font-extrabold text-text-main truncate">{metrics ? metrics.pending_verifications || 0 : '...'}</p>
            </div>
          </button>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 md:gap-4 mb-8">
          <button 
            onClick={() => navigate('/owner/finance')}
            className="bg-white p-4 md:p-5 rounded-2xl border border-border-main shadow-sm flex items-center gap-3 md:gap-4 hover:border-purple-500 hover:shadow-md transition-all text-left w-full"
          >
            <div className="w-10 h-10 md:w-12 md:h-12 bg-purple-50 text-purple-600 rounded-xl flex items-center justify-center shrink-0">
              <Wallet className="w-5 h-5 md:w-6 md:h-6" />
            </div>
            <div className="min-w-0">
              <p className="text-text-muted font-bold text-[11px] md:text-xs truncate">Pendapatan Booking</p>
              <p className="text-base sm:text-lg font-extrabold text-text-main truncate">
                {metrics ? new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 }).format(metrics.booking_revenue_current ?? metrics.revenue_current ?? 0) : '...'}
              </p>
            </div>
          </button>

          <button 
            onClick={() => navigate('/owner/finance')}
            className="bg-white p-4 md:p-5 rounded-2xl border border-border-main shadow-sm flex items-center gap-3 md:gap-4 hover:border-red-500 hover:shadow-md transition-all text-left w-full"
          >
            <div className="w-10 h-10 md:w-12 md:h-12 bg-red-50 text-red-600 rounded-xl flex items-center justify-center shrink-0">
              <Wallet className="w-5 h-5 md:w-6 md:h-6" />
            </div>
            <div className="min-w-0">
              <p className="text-text-muted font-bold text-[11px] md:text-xs truncate">Refund</p>
              <p className="text-base sm:text-lg font-extrabold text-text-main truncate">
                {metrics ? new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 }).format(metrics.refund_current ?? 0) : '...'}
              </p>
            </div>
          </button>

          <button 
            onClick={() => navigate('/owner/finance')}
            className="bg-white p-4 md:p-5 rounded-2xl border border-border-main shadow-sm flex items-center gap-3 md:gap-4 hover:border-blue-500 hover:shadow-md transition-all text-left w-full"
          >
            <div className="w-10 h-10 md:w-12 md:h-12 bg-blue-50 text-blue-600 rounded-xl flex items-center justify-center shrink-0">
              <Wallet className="w-5 h-5 md:w-6 md:h-6" />
            </div>
            <div className="min-w-0">
              <p className="text-text-muted font-bold text-[11px] md:text-xs truncate">Pendapatan Bersih Booking</p>
              <p className="text-base sm:text-lg font-extrabold text-text-main truncate">
                {metrics ? new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 }).format(metrics.net_revenue_current ?? ((metrics.booking_revenue_current ?? metrics.revenue_current ?? 0) - (metrics.refund_current ?? 0))) : '...'}
              </p>
            </div>
          </button>
        </div>
        <p className="text-xs text-text-muted mt-2 mb-8 flex items-center justify-center gap-1 font-medium bg-blue-50/50 p-2 rounded-lg max-w-2xl mx-auto w-fit">
          Tidak termasuk pemasukan manual seperti sponsor. Lihat <Link to="/owner/finance" className="text-blue-600 font-bold hover:underline">halaman Keuangan</Link> untuk total kas.
        </p>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-8">
          {/* Left Column: Profile */}
          <div className="md:col-span-1">
            <div className="bg-white rounded-3xl border border-border-main p-6 shadow-sm">
              <div className="flex items-center gap-3 mb-6 pb-4 border-b border-border-main">
                <User className="w-5 h-5 text-primary" />
                <h2 className="font-extrabold text-lg text-text-main">Profil Bisnis</h2>
              </div>
              
              {profile ? (
                <div className="space-y-4">
                  <div>
                    <p className="text-xs font-bold text-text-muted">Nama Bisnis</p>
                    <p className="font-bold text-text-main">{profile.business_name}</p>
                  </div>
                  <div>
                    <p className="text-xs font-bold text-text-muted">No. Telepon</p>
                    <p className="font-bold text-text-main">{profile.phone_number}</p>
                  </div>
                  <div>
                    <p className="text-[11px] md:text-xs font-bold text-text-muted">Rekening Bank</p>
                    {profile.bank_account_number ? (
                      <>
                        <p className="font-bold text-text-main text-sm md:text-base">{profile.bank_name} - {profile.bank_account_number}</p>
                        <p className="text-xs md:text-sm text-text-muted">a.n {profile.bank_account_name}</p>
                      </>
                    ) : (
                      <p className="text-sm italic text-text-muted mt-0.5">Belum ditambahkan</p>
                    )}
                  </div>
                </div>
              ) : (
                <div className="text-center py-6">
                  <p className="text-text-muted text-sm mb-4">Profil bisnis belum dilengkapi.</p>
                  <button className="text-primary font-bold hover:underline text-sm">Lengkapi Profil</button>
                </div>
              )}
            </div>
          </div>

          {/* Right Column: Quick Actions */}
          <div className="md:col-span-2">
            <h2 className="font-extrabold text-xl text-text-main mb-4">Aksi Cepat</h2>
            <div className="flex flex-col gap-3">
              <button 
                onClick={() => navigate('/owner/venues')}
                className="bg-white p-3 md:p-4 rounded-xl border border-border-main shadow-sm hover:border-primary hover:shadow-md transition-all text-left flex items-center justify-between group w-full"
              >
                <div className="flex items-center gap-3 md:gap-4 min-w-0">
                  <div className="w-10 h-10 bg-primary/10 text-primary rounded-xl flex items-center justify-center shrink-0 group-hover:scale-110 transition-transform">
                    <Building2 className="w-5 h-5" />
                  </div>
                  <div>
                    <h3 className="font-extrabold text-base text-text-main">Manajemen Venue</h3>
                    <p className="text-text-muted text-xs">Atur informasi, fasilitas, dan detail lokasi venue Anda.</p>
                  </div>
                </div>
                <ArrowRight className="w-4 h-4 text-primary opacity-0 -translate-x-2 group-hover:opacity-100 group-hover:translate-x-0 transition-all" />
              </button>

              <button 
                onClick={() => navigate('/owner/bookings')}
                className="bg-white p-3 md:p-4 rounded-xl border border-border-main shadow-sm hover:border-secondary hover:shadow-md transition-all text-left flex items-center justify-between group w-full"
              >
                <div className="flex items-center gap-3 md:gap-4 min-w-0">
                  <div className="w-10 h-10 bg-secondary/10 text-secondary rounded-xl flex items-center justify-center shrink-0 group-hover:scale-110 transition-transform">
                    <CalendarDays className="w-5 h-5" />
                  </div>
                  <div>
                    <h3 className="font-extrabold text-base text-text-main">Pesanan Masuk</h3>
                    <p className="text-text-muted text-xs">Kelola semua pesanan dari semua venue Anda.</p>
                  </div>
                </div>
                <ArrowRight className="w-4 h-4 text-secondary opacity-0 -translate-x-2 group-hover:opacity-100 group-hover:translate-x-0 transition-all" />
              </button>
            </div>
          </div>
        </div>

        {/* Charts Section */}
        <div className="mb-6 mt-12 flex flex-col md:flex-row md:items-end justify-between gap-2 border-b border-border-main pb-4">
          <div>
            <h2 className="font-extrabold text-xl text-text-main">Performa Periode Ini</h2>
            <p className="text-sm font-medium text-text-muted mt-1">Data pada grafik menyesuaikan filter tanggal</p>
          </div>
          {startDate && endDate && (
            <div className="bg-primary/5 px-4 py-2 rounded-lg border border-primary/20 text-primary font-bold text-sm">
              Periode: {new Date(startDate).toLocaleDateString('id-ID', { day: 'numeric', month: 'long', year: 'numeric' })} - {new Date(endDate).toLocaleDateString('id-ID', { day: 'numeric', month: 'long', year: 'numeric' })}
            </div>
          )}
        </div>
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-8 animate-slide-up" style={{ animationDelay: '0.4s' }}>
            <BookingsChart data={bookingsTrend.map(i => ({ date: i.date, count: i.booking_count }))} isError={trendError} />
            <RevenueChart data={revenueBreakdown} isError={revenueError} subtitle="Hanya menghitung transaksi booking. Pemasukan manual seperti sponsor tidak termasuk." />
          </div>
      </div>
    </PageShell>
  );
};
