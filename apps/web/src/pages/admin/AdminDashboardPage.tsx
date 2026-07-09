import React from 'react';

export const AdminDashboardPage: React.FC = () => {
  return (
    <div>
      <h1 className="text-2xl font-bold text-slate-900 mb-6">Superadmin Dashboard</h1>
      <div className="bg-white p-6 rounded-xl border border-slate-200 shadow-sm">
        <p className="text-slate-600 mb-4">
          Selamat datang di halaman dashboard Superadmin. Gunakan navigasi di samping untuk mengelola pengguna, pemilik venue, venue, dan melihat log audit platform.
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
