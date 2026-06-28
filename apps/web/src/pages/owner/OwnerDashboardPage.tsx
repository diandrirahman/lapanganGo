import React, { useEffect, useState, useCallback } from 'react';
import { PageShell } from '../../components/layout/PageShell';
import { useAuth } from '../../contexts/AuthContext';
import { useNavigate } from 'react-router-dom';
import { fetchOwnerProfile, fetchOwnerMetrics } from '../../lib/api';
import type { OwnerProfile, OwnerMetrics } from '../../types/owner';
import { Building2, CalendarDays, Wallet, User, ArrowRight, AlertCircle } from 'lucide-react';

export const OwnerDashboardPage: React.FC = () => {
  const { token } = useAuth();
  const [profile, setProfile] = useState<OwnerProfile | null>(null);
  const [metrics, setMetrics] = useState<OwnerMetrics | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [startDate, setStartDate] = useState('');
  const [endDate, setEndDate] = useState('');
  const navigate = useNavigate();

  const loadDashboard = useCallback(() => {
    if (token) {
      setIsLoading(true);
      Promise.all([
        fetchOwnerProfile(token).then(setProfile).catch(console.error),
        fetchOwnerMetrics(token, startDate, endDate).then(setMetrics).catch(console.error)
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
        <div className="flex justify-between items-end mb-8">
          <div>
            <h1 className="text-3xl md:text-4xl font-extrabold text-text-main mb-2">Dashboard Owner</h1>
            <p className="text-text-muted text-lg">Selamat datang kembali, kelola bisnis lapangan Anda di sini.</p>
          </div>
          
          <div className="flex gap-4">
            <div>
              <label className="block text-xs font-bold text-text-muted mb-1">Mulai Tanggal</label>
              <input 
                type="date"
                value={startDate}
                onChange={(e) => setStartDate(e.target.value)}
                className="px-3 py-2 rounded-xl border border-border-main text-sm outline-none focus:border-primary focus:ring-1 focus:ring-primary"
              />
            </div>
            <div>
              <label className="block text-xs font-bold text-text-muted mb-1">Sampai Tanggal</label>
              <input 
                type="date"
                value={endDate}
                onChange={(e) => setEndDate(e.target.value)}
                className="px-3 py-2 rounded-xl border border-border-main text-sm outline-none focus:border-primary focus:ring-1 focus:ring-primary"
              />
            </div>
          </div>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
          {/* Stats Cards */}
          <div className="bg-white p-5 rounded-2xl border border-border-main shadow-sm flex items-center gap-4">
            <div className="w-12 h-12 bg-blue-50 text-blue-600 rounded-xl flex items-center justify-center shrink-0">
              <Building2 className="w-6 h-6" />
            </div>
            <div>
              <p className="text-text-muted font-bold text-xs">Total Venue</p>
              <p className="text-xl font-extrabold text-text-main">{metrics ? metrics.total_venues : '...'}</p>
            </div>
          </div>
          
          <button 
            onClick={() => navigate('/owner/venues?intent=upcoming_bookings')}
            className="bg-white p-5 rounded-2xl border border-border-main shadow-sm flex items-center gap-4 hover:border-green-300 hover:shadow-md transition-all text-left"
          >
            <div className="w-12 h-12 bg-green-50 text-green-600 rounded-xl flex items-center justify-center shrink-0">
              <CalendarDays className="w-6 h-6" />
            </div>
            <div>
              <p className="text-text-muted font-bold text-xs">Pesanan Mendatang</p>
              <p className="text-xl font-extrabold text-text-main">{metrics ? metrics.upcoming_bookings : '...'}</p>
            </div>
          </button>

          <div className="bg-white p-5 rounded-2xl border border-border-main shadow-sm flex items-center gap-4">
            <div className="w-12 h-12 bg-yellow-50 text-yellow-600 rounded-xl flex items-center justify-center shrink-0">
              <AlertCircle className="w-6 h-6" />
            </div>
            <div>
              <p className="text-text-muted font-bold text-xs">Menunggu Verifikasi</p>
              <p className="text-xl font-extrabold text-text-main">{metrics ? metrics.pending_verifications || 0 : '...'}</p>
            </div>
          </div>

          <div className="bg-white p-5 rounded-2xl border border-border-main shadow-sm flex items-center gap-4">
            <div className="w-12 h-12 bg-purple-50 text-purple-600 rounded-xl flex items-center justify-center shrink-0">
              <Wallet className="w-6 h-6" />
            </div>
            <div>
              <p className="text-text-muted font-bold text-xs">Pendapatan (Periode)</p>
              <p className="text-lg font-extrabold text-text-main">
                {metrics ? new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 }).format(metrics.revenue_current) : '...'}
              </p>
            </div>
          </div>
        </div>

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
                    <p className="text-xs font-bold text-text-muted">Rekening Bank</p>
                    <p className="font-bold text-text-main">{profile.bank_name} - {profile.bank_account_number}</p>
                    <p className="text-sm text-text-muted">a.n {profile.bank_account_name}</p>
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
                className="bg-white p-4 rounded-2xl border border-border-main shadow-sm hover:border-primary hover:shadow-md transition-all text-left flex items-center justify-between group"
              >
                <div className="flex items-center gap-4">
                  <div className="w-10 h-10 bg-primary/10 text-primary rounded-xl flex items-center justify-center group-hover:scale-110 transition-transform">
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
                onClick={() => navigate('/owner/venues?intent=bookings')}
                className="bg-white p-4 rounded-2xl border border-border-main shadow-sm hover:border-secondary hover:shadow-md transition-all text-left flex items-center justify-between group"
              >
                <div className="flex items-center gap-4">
                  <div className="w-10 h-10 bg-secondary/10 text-secondary rounded-xl flex items-center justify-center group-hover:scale-110 transition-transform">
                    <CalendarDays className="w-5 h-5" />
                  </div>
                  <div>
                    <h3 className="font-extrabold text-base text-text-main">Pesanan Masuk</h3>
                    <p className="text-text-muted text-xs">Pilih venue untuk melihat dan memverifikasi pesanan.</p>
                  </div>
                </div>
                <ArrowRight className="w-4 h-4 text-secondary opacity-0 -translate-x-2 group-hover:opacity-100 group-hover:translate-x-0 transition-all" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </PageShell>
  );
};
