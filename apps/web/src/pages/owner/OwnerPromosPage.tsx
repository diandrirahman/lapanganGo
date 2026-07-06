import { useState, useEffect, useCallback } from 'react';
import { useAuth } from '../../contexts/AuthContext';
import { promoService } from '../../services/promoService';
import type { Promo } from '../../types/promo';
import toast from 'react-hot-toast';
import { PromoModal } from '../../components/owner/PromoModal';
import { Calendar, Tag, Activity, Plus } from 'lucide-react';
import { formatRupiah } from '../../lib/utils';
import { PageShell } from '../../components/layout/PageShell';
import { LoadingState } from '../../components/feedback/LoadingState';
import { ErrorState } from '../../components/feedback/ErrorState';

const formatShortDate = (value: string) => {
  return new Intl.DateTimeFormat('id-ID', {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
  }).format(new Date(value));
};

export function OwnerPromosPage() {
  const { token } = useAuth();
  const [promos, setPromos] = useState<Promo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [selectedPromo, setSelectedPromo] = useState<Promo | undefined>();

  const loadPromos = useCallback(async () => {
    try {
      setError(null);
      if (token) {
        const data = await promoService.getPromos(token);
        setPromos(Array.isArray(data) ? data : []);
      } else {
        setPromos([]);
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Gagal memuat data promo';
      setError(message);
      toast.error(message);
    } finally {
      setLoading(false);
    }
  }, [token]);

  useEffect(() => {
    loadPromos();
  }, [loadPromos]);

  const handleToggleStatus = async (promoId: string) => {
    try {
      if (token) {
        await promoService.togglePromo(token, promoId);
        toast.success('Status promo berhasil diubah');
        loadPromos();
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Gagal mengubah status promo';
      toast.error(message);
    }
  };

  const openAddModal = () => {
    setSelectedPromo(undefined);
    setIsModalOpen(true);
  };

  const openEditModal = (promo: Promo) => {
    setSelectedPromo(promo);
    setIsModalOpen(true);
  };

  const handleDeletePromo = async (promoId: string) => {
    if (!window.confirm('Apakah Anda yakin ingin menghapus promo ini?')) return;
    try {
      if (token) {
        await promoService.deletePromo(token, promoId);
        toast.success('Promo berhasil dihapus');
        loadPromos();
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Gagal menghapus promo';
      toast.error(message);
    }
  };

  if (loading) {
    return (
      <PageShell>
        <div className="pt-24 pb-40 max-w-6xl mx-auto px-6">
          <LoadingState message="Memuat data promo..." />
        </div>
      </PageShell>
    );
  }

  return (
    <PageShell>
      <div className="pt-24 pb-40 max-w-6xl mx-auto px-6 space-y-6">
      {error && (
        <ErrorState message={error} onRetry={loadPromos} />
      )}

      <div className="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-4 bg-white p-6 rounded-2xl border border-border-main shadow-sm">
        <div>
          <h1 className="text-3xl font-extrabold text-text-main mb-2">Manajemen Promo</h1>
          <p className="text-text-muted">Buat dan atur kode promo untuk menarik lebih banyak pelanggan.</p>
        </div>
        <button
          onClick={openAddModal}
          className="flex items-center justify-center gap-2 bg-primary hover:bg-primary/90 text-white px-5 py-3 rounded-xl font-bold transition-colors"
        >
          <Plus size={20} />
          <span>Buat Promo</span>
        </button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
        {promos.map((promo) => (
          <div key={promo.id} className="bg-white rounded-2xl border border-border-main p-6 flex flex-col shadow-sm hover:border-primary/30 transition-colors">
            <div className="flex justify-between items-start mb-4">
              <div>
                <span className="inline-block px-3 py-1 bg-primary/10 text-primary font-bold rounded-lg mb-2 tracking-wider">
                  {promo.code}
                </span>
                <h3 className="text-lg font-bold text-text-main line-clamp-1">{promo.name}</h3>
              </div>
              <button
                onClick={() => handleToggleStatus(promo.id)}
                className={`px-3 py-1 text-xs font-medium rounded-full ${
                  promo.status === 'ACTIVE'
                    ? 'bg-emerald-50 text-emerald-700 hover:bg-emerald-100'
                    : 'bg-gray-100 text-text-muted hover:bg-gray-200'
                } transition-colors`}
              >
                {promo.status === 'ACTIVE' ? 'Aktif' : 'Tidak Aktif'}
              </button>
            </div>

            {promo.description && (
              <p className="text-text-muted text-sm mb-4 line-clamp-2 flex-grow">{promo.description}</p>
            )}

            <div className="space-y-3 mt-auto">
              <div className="flex items-center gap-3 text-sm">
                <div className="w-8 h-8 rounded-full bg-blue-500/10 flex items-center justify-center flex-shrink-0">
                  <Tag size={16} className="text-blue-400" />
                </div>
                <div>
                  <p className="text-text-muted text-xs">Nilai Diskon</p>
                  <p className="text-text-main font-medium">
                    {promo.discount_type === 'PERCENTAGE' 
                      ? `${promo.discount_value}%` 
                      : formatRupiah(promo.discount_value)}
                  </p>
                </div>
              </div>

              <div className="flex items-center gap-3 text-sm">
                <div className="w-8 h-8 rounded-full bg-purple-500/10 flex items-center justify-center flex-shrink-0">
                  <Calendar size={16} className="text-purple-400" />
                </div>
                <div>
                  <p className="text-text-muted text-xs">Berlaku untuk jadwal main</p>
                  <p className="text-text-main font-medium">
                    {formatShortDate(promo.starts_at)} - {formatShortDate(promo.ends_at)}
                  </p>
                </div>
              </div>

              <div className="flex items-center gap-3 text-sm">
                <div className="w-8 h-8 rounded-full bg-gray-100 flex items-center justify-center flex-shrink-0">
                  <Activity size={16} className="text-text-muted" />
                </div>
                <div className="flex-grow">
                  <p className="text-text-muted text-xs">Status Promo</p>
                  <p className={`font-medium ${
                    new Date() < new Date(promo.starts_at) ? 'text-amber-600' :
                    new Date() > new Date(promo.ends_at) ? 'text-red-600' :
                    promo.status === 'ACTIVE' ? 'text-emerald-600' : 'text-slate-500'
                  }`}>
                    {new Date() < new Date(promo.starts_at) ? 'Belum Dimulai' :
                     new Date() > new Date(promo.ends_at) ? 'Kedaluwarsa' :
                     promo.status === 'ACTIVE' ? 'Sedang Berjalan' : 'Nonaktif'}
                  </p>
                </div>
              </div>
            </div>

            <div className="mt-6 flex gap-3">
              <button
                onClick={() => openEditModal(promo)}
                className="flex-1 py-2 bg-primary/10 hover:bg-primary/15 text-primary font-bold rounded-xl transition-colors"
              >
                Edit Promo
              </button>
              <button
                onClick={() => handleDeletePromo(promo.id)}
                className="px-4 py-2 bg-red-500/10 hover:bg-red-500/20 text-red-500 font-medium rounded-xl transition-colors"
              >
                Hapus
              </button>
            </div>
          </div>
        ))}

        {promos.length === 0 && (
          <div className="col-span-full py-16 text-center border-2 border-dashed border-border-main rounded-2xl bg-white">
            <div className="w-16 h-16 bg-primary/10 rounded-full flex items-center justify-center mx-auto mb-4">
              <Tag size={32} className="text-primary" />
            </div>
            <h3 className="text-lg font-bold text-text-main mb-2">Belum ada promo</h3>
            <p className="text-text-muted mb-6 max-w-sm mx-auto">
              Buat kode promo pertama Anda untuk memberikan penawaran spesial kepada pelanggan.
            </p>
            <button
              onClick={openAddModal}
              className="bg-primary hover:bg-primary/90 text-white px-6 py-2.5 rounded-xl font-bold transition-colors"
            >
              Buat Promo Sekarang
            </button>
          </div>
        )}
      </div>

      <PromoModal
        isOpen={isModalOpen}
        onClose={() => setIsModalOpen(false)}
        onSuccess={loadPromos}
        promo={selectedPromo}
      />
    </div>
    </PageShell>
  );
}
