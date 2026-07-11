import React, { useEffect, useState } from 'react';
import { adminApi } from '../../lib/api/admin';
import type { DashboardStatsResponse } from '../../lib/api/admin';
import toast from 'react-hot-toast';
import { Users, Building2, MapPin, CalendarDays, RefreshCw } from 'lucide-react';

export const AdminDashboardPage: React.FC = () => {
  const [stats, setStats] = useState<DashboardStatsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchStats = async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await adminApi.getDashboardStats();
      setStats(data);
    } catch (err: any) {
      const errMsg = err.message || 'Failed to fetch dashboard stats';
      setError(errMsg);
      toast.error(errMsg);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchStats();
  }, []);

  return (
    <div className="space-y-6 max-w-7xl mx-auto">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">Superadmin Dashboard</h1>
          <p className="text-sm text-slate-500 mt-1">Platform overview and statistics</p>
        </div>
        <button
          onClick={fetchStats}
          disabled={loading}
          className="inline-flex items-center justify-center px-4 py-2 bg-white border border-slate-200 rounded-lg text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-50 transition-colors"
        >
          <RefreshCw className={`mr-2 h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
          Refresh
        </button>
      </div>

      {error && !stats ? (
        <div role="alert" className="bg-red-50 border border-red-200 text-red-700 p-6 rounded-lg flex flex-col items-center justify-center space-y-4">
          <p className="font-medium">{error}</p>
          <button
            onClick={fetchStats}
            className="px-4 py-2 bg-white border border-red-200 rounded-lg text-sm font-medium text-red-700 hover:bg-red-100 transition-colors"
          >
            Retry
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
          <div className="bg-white p-6 rounded-xl border border-slate-200 shadow-sm flex items-center space-x-4">
            <div className="p-3 bg-blue-50 text-blue-600 rounded-lg">
              <Users className="h-6 w-6" />
            </div>
            <div>
              <p className="text-sm font-medium text-slate-500">Total Users</p>
              <p className="text-2xl font-bold text-slate-900">{loading && !stats ? '-' : stats?.total_users ?? '-'}</p>
            </div>
          </div>
          <div className="bg-white p-6 rounded-xl border border-slate-200 shadow-sm flex items-center space-x-4">
            <div className="p-3 bg-emerald-50 text-emerald-600 rounded-lg">
              <Building2 className="h-6 w-6" />
            </div>
            <div>
              <p className="text-sm font-medium text-slate-500">Total Owners</p>
              <p className="text-2xl font-bold text-slate-900">{loading && !stats ? '-' : stats?.total_owners ?? '-'}</p>
            </div>
          </div>
          <div className="bg-white p-6 rounded-xl border border-slate-200 shadow-sm flex items-center space-x-4">
            <div className="p-3 bg-purple-50 text-purple-600 rounded-lg">
              <MapPin className="h-6 w-6" />
            </div>
            <div>
              <p className="text-sm font-medium text-slate-500">Total Venues</p>
              <p className="text-2xl font-bold text-slate-900">{loading && !stats ? '-' : stats?.total_venues ?? '-'}</p>
            </div>
          </div>
          <div className="bg-white p-6 rounded-xl border border-slate-200 shadow-sm flex items-center space-x-4">
            <div className="p-3 bg-amber-50 text-amber-600 rounded-lg">
              <CalendarDays className="h-6 w-6" />
            </div>
            <div>
              <p className="text-sm font-medium text-slate-500">Total Bookings</p>
              <p className="text-2xl font-bold text-slate-900">{loading && !stats ? '-' : stats?.total_bookings ?? '-'}</p>
            </div>
          </div>
        </div>
      )}

      {error && stats && (
        <div role="status" className="flex flex-col gap-3 border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900 sm:flex-row sm:items-center sm:justify-between rounded-lg">
          <p>Menampilkan data terakhir. Statistik terbaru gagal dimuat: {error}</p>
          <button
            onClick={fetchStats}
            disabled={loading}
            className="shrink-0 px-4 py-2 bg-white border border-amber-300 rounded-lg font-medium hover:bg-amber-100 disabled:opacity-50 transition-colors"
          >
            Coba Lagi
          </button>
        </div>
      )}

      <div className="bg-white p-6 rounded-xl border border-slate-200 shadow-sm">
        <p className="text-slate-600 mb-4">
          Gunakan navigasi di samping untuk mengelola pengguna, pemilik venue, venue, dan melihat log audit platform.
        </p>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-6 mt-8">
          <div className="p-6 bg-slate-50 border border-slate-100 rounded-lg">
            <h3 className="text-sm font-medium text-slate-500 mb-1">Manajemen Pengguna</h3>
            <p className="text-sm text-slate-700">Lihat semua akun terdaftar di platform, termasuk admin, pemilik venue, staf, dan pengguna biasa.</p>
          </div>
          <div className="p-6 bg-slate-50 border border-slate-100 rounded-lg">
            <h3 className="text-sm font-medium text-slate-500 mb-1">Manajemen Venue</h3>
            <p className="text-sm text-slate-700">Awasi pendaftaran venue dan pemiliknya. Tangguhkan akses jika ditemukan pelanggaran.</p>
          </div>
          <div className="p-6 bg-slate-50 border border-slate-100 rounded-lg">
            <h3 className="text-sm font-medium text-slate-500 mb-1">Audit Logs</h3>
            <p className="text-sm text-slate-700">Lacak setiap perubahan penting yang terjadi di sistem untuk keamanan dan transparansi.</p>
          </div>
        </div>
      </div>
    </div>
  );
};
