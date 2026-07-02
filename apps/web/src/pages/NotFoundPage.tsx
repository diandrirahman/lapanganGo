import React from 'react';
import { PageShell } from '../components/layout/PageShell';
import { Button } from '../components/ui/Button';
import { Link } from 'react-router-dom';
import { Compass } from 'lucide-react';

export const NotFoundPage: React.FC = () => {
  return (
    <PageShell>
      <div className="flex flex-col items-center justify-center min-h-[60vh] px-6 text-center">
        <div className="bg-surface p-6 rounded-full mb-6 text-primary shadow-sm border border-gray-100">
          <Compass className="w-16 h-16" />
        </div>
        <h1 className="text-4xl md:text-5xl font-extrabold text-text-main mb-4 tracking-tight">
          404 - Halaman Tidak Ditemukan
        </h1>
        <p className="text-lg text-text-muted max-w-md mb-8">
          Maaf, halaman yang Anda cari tidak ada atau telah dipindahkan.
        </p>
        <div className="flex gap-4 flex-col sm:flex-row">
          <Link to="/">
            <Button variant="outline" className="w-full sm:w-auto px-8 font-bold border-border-main">
              Kembali ke Beranda
            </Button>
          </Link>
          <Link to="/venues">
            <Button className="w-full sm:w-auto px-8 bg-primary text-white font-bold shadow-sm border-none">
              Cari Lapangan
            </Button>
          </Link>
        </div>
      </div>
    </PageShell>
  );
};
