import { useState, useEffect } from 'react';
import { useAuth } from '../../contexts/AuthContext';
import { promoService } from '../../services/promoService';
import type { Promo, CreatePromoRequest } from '../../types/promo';
import toast from 'react-hot-toast';
import { X, Calendar, Tag, Info, FileText } from 'lucide-react';

import { fetchOwnerVenues } from '../../lib/api';
import type { Venue } from '../../types/venue';

interface PromoModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess: () => void;
  promo?: Promo;
}

export function PromoModal({ isOpen, onClose, onSuccess, promo }: PromoModalProps) {
  const { token, user } = useAuth();
  const [loading, setLoading] = useState(false);
  const [venues, setVenues] = useState<Venue[]>([]);
  const [formData, setFormData] = useState<CreatePromoRequest & { venue_id?: string }>({
    code: '',
    name: '',
    description: '',
    discount_type: 'PERCENTAGE',
    discount_value: 0,
    starts_at: '',
    ends_at: '',
    status: 'ACTIVE'
  });

  useEffect(() => {
    if (!isOpen) return;

    if (token) {
      fetchOwnerVenues(token).then(data => {
        setVenues(data);
        if (user?.role === 'STAFF' && data.length > 0 && !promo) {
          setFormData(prev => ({ ...prev, venue_id: data[0].id }));
        }
      }).catch(console.error);
    }
    if (promo) {
      setFormData({
        code: promo.code,
        name: promo.name,
        description: promo.description || '',
        discount_type: promo.discount_type,
        discount_value: promo.discount_value,
        starts_at: new Date(promo.starts_at).toISOString().slice(0, 16),
        ends_at: new Date(promo.ends_at).toISOString().slice(0, 16),
        status: promo.status,
        venue_id: promo.venue_id || ''
      });
    } else {
      // Default to tomorrow 00:00 to next month
      const start = new Date();
      start.setDate(start.getDate() + 1);
      start.setHours(0, 0, 0, 0);
      
      const end = new Date(start);
      end.setMonth(end.getMonth() + 1);
      
      // We subtract timezone offset because toISOString gives UTC but local datetime picker expects local time format
      const tzOffset = new Date().getTimezoneOffset() * 60000; // offset in milliseconds
      
      const localStart = new Date(start.getTime() - tzOffset);
      const localEnd = new Date(end.getTime() - tzOffset);

      setFormData({
        code: '',
        name: '',
        description: '',
        discount_type: 'PERCENTAGE',
        discount_value: 10,
        starts_at: localStart.toISOString().slice(0, 16),
        ends_at: localEnd.toISOString().slice(0, 16),
        status: 'ACTIVE',
        venue_id: ''
      });
    }
  }, [promo, isOpen, token, user?.role]);

  if (!isOpen) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token) return;

    if (new Date(formData.ends_at) <= new Date(formData.starts_at)) {
      toast.error('Waktu berakhir harus setelah waktu mulai');
      return;
    }

    if (formData.discount_type === 'PERCENTAGE' && (formData.discount_value <= 0 || formData.discount_value >= 100)) {
      toast.error('Diskon persentase harus antara 1-99');
      return;
    }

    if (formData.discount_type === 'FIXED_AMOUNT' && formData.discount_value <= 0) {
      toast.error('Nominal diskon harus lebih dari 0');
      return;
    }

    try {
      setLoading(true);
      // Ensure ISO string conversion uses the proper timezone (append Z if we treat input as UTC, or construct properly)
      // Since <input type="datetime-local"> creates local time, we just parse it as Date and toISOString it.
      const payload: CreatePromoRequest = {
        ...formData,
        starts_at: new Date(formData.starts_at).toISOString(),
        ends_at: new Date(formData.ends_at).toISOString(),
      };
      if (!payload.venue_id) {
        delete payload.venue_id;
      }

      if (promo) {
        await promoService.updatePromo(token, promo.id, payload);
        toast.success('Promo berhasil diupdate');
      } else {
        await promoService.createPromo(token, payload);
        toast.success('Promo berhasil dibuat');
      }
      onSuccess();
      onClose();
    } catch (error: any) {
      toast.error(error.message || 'Terjadi kesalahan saat menyimpan promo');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/50 backdrop-blur-sm">
      <div className="bg-white rounded-2xl w-full max-w-2xl border border-border-main shadow-xl flex flex-col max-h-[90vh]">
        <div className="p-6 border-b border-border-main flex justify-between items-center shrink-0">
          <div>
            <h2 className="text-xl font-bold text-text-main">
              {promo ? 'Edit Promo' : 'Buat Promo Baru'}
            </h2>
            <p className="text-text-muted text-sm mt-1">
              Atur kode promo dan detail diskon
            </p>
          </div>
          <button
            onClick={onClose}
            className="p-2 text-text-muted hover:text-text-main hover:bg-slate-100 rounded-full transition-colors"
          >
            <X size={20} />
          </button>
        </div>

        <div className="p-6 overflow-y-auto custom-scrollbar">
          <form id="promoForm" onSubmit={handleSubmit} className="space-y-8">
            
            {/* Informasi Promo */}
            <div>
              <h3 className="text-sm font-semibold text-text-main mb-4 flex items-center gap-2">
                <FileText size={16} className="text-primary" />
                Informasi Promo
              </h3>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-6 mb-6">
                <div className="space-y-2">
                  <label className="text-sm font-medium text-text-main flex items-center gap-2">
                    <Tag size={16} className="text-primary" />
                    Kode Promo
                  </label>
                  <input
                    type="text"
                    required
                    disabled={!!promo}
                    maxLength={50}
                    className="w-full bg-white border border-border-main rounded-xl px-4 py-3 text-text-main placeholder:text-text-muted focus:outline-none focus:ring-2 focus:ring-primary/20 focus:border-primary disabled:opacity-50 uppercase"
                    placeholder="Contoh: MERDEKA20"
                    value={formData.code}
                    onChange={e => setFormData(prev => ({ ...prev, code: e.target.value.toUpperCase().replace(/\s/g, '') }))}
                  />
                  {!promo && (
                    <p className="text-xs text-text-muted">Gunakan huruf dan angka tanpa spasi.</p>
                  )}
                </div>

                <div className="space-y-2">
                  <label className="text-sm font-medium text-text-main flex items-center gap-2">
                    <Info size={16} className="text-primary" />
                    Nama Promo
                  </label>
                  <input
                    type="text"
                    required
                    className="w-full bg-white border border-border-main rounded-xl px-4 py-3 text-text-main placeholder:text-text-muted focus:outline-none focus:ring-2 focus:ring-primary/20 focus:border-primary"
                    placeholder="Contoh: Promo Spesial Kemerdekaan"
                    value={formData.name}
                    onChange={e => setFormData(prev => ({ ...prev, name: e.target.value }))}
                  />
                </div>
              </div>

              <div className="space-y-2">
                <label className="text-sm font-medium text-text-main flex items-center gap-2">
                  Deskripsi
                </label>
                <textarea
                  rows={3}
                  className="w-full bg-white border border-border-main rounded-xl px-4 py-3 text-text-main placeholder:text-text-muted focus:outline-none focus:ring-2 focus:ring-primary/20 focus:border-primary resize-none"
                  placeholder="Penjelasan singkat mengenai promo ini (Opsional)"
                  value={formData.description}
                  onChange={e => setFormData(prev => ({ ...prev, description: e.target.value }))}
                />
              </div>
            </div>

            {/* Cakupan Venue */}
            <div className="pt-6 border-t border-border-main">
              <div className="space-y-2">
                <label className="text-sm font-medium text-text-main flex items-center gap-2">
                  <Info size={16} className="text-primary" />
                  Cakupan Venue (Opsional)
                </label>
                <select
                  className="w-full bg-white border border-border-main rounded-xl px-4 py-3 text-text-main focus:outline-none focus:ring-2 focus:ring-primary/20 focus:border-primary disabled:opacity-50"
                  value={formData.venue_id || ''}
                  onChange={e => setFormData(prev => ({ ...prev, venue_id: e.target.value }))}
                  disabled={!!promo}
                >
                  {user?.role !== 'STAFF' && <option value="">Semua Venue (Global)</option>}
                  {venues.map(v => (
                    <option key={v.id} value={v.id}>{v.name}</option>
                  ))}
                </select>
                {!promo && (
                  <p className="text-xs text-text-muted">Pilih venue jika promo ini khusus untuk venue tertentu.</p>
                )}
              </div>
            </div>

            {/* Pengaturan Diskon */}
            <div className="pt-6 border-t border-border-main">
              <h3 className="text-sm font-semibold text-text-main mb-4 flex items-center gap-2">
                <Tag size={16} className="text-primary" />
                Pengaturan Diskon
              </h3>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div className="space-y-2">
                  <label className="text-sm font-medium text-text-main">Tipe Diskon</label>
                  <select
                    className="w-full bg-white border border-border-main rounded-xl px-4 py-3 text-text-main focus:outline-none focus:ring-2 focus:ring-primary/20 focus:border-primary"
                    value={formData.discount_type}
                    onChange={e => setFormData(prev => ({ 
                      ...prev, 
                      discount_type: e.target.value as 'PERCENTAGE' | 'FIXED_AMOUNT',
                      discount_value: e.target.value === 'PERCENTAGE' ? 10 : 10000
                    }))}
                  >
                    <option value="PERCENTAGE">Persentase (%)</option>
                    <option value="FIXED_AMOUNT">Nominal Tetap (Rp)</option>
                  </select>
                </div>

                <div className="space-y-2">
                  <label className="text-sm font-medium text-text-main">Nilai Diskon</label>
                  <div className="relative">
                    <input
                      type="number"
                      required
                      min={1}
                      max={formData.discount_type === 'PERCENTAGE' ? 99 : undefined}
                      className={`w-full bg-white border border-border-main rounded-xl py-3 text-text-main focus:outline-none focus:ring-2 focus:ring-primary/20 focus:border-primary ${
                        formData.discount_type === 'FIXED_AMOUNT' ? 'pl-10 pr-4' : 'px-4'
                      }`}
                      value={formData.discount_value || ''}
                      onChange={e => setFormData(prev => ({ ...prev, discount_value: Number(e.target.value) }))}
                    />
                    {formData.discount_type === 'FIXED_AMOUNT' && (
                      <span className="absolute left-4 top-1/2 -translate-y-1/2 text-text-muted">Rp</span>
                    )}
                    {formData.discount_type === 'PERCENTAGE' && (
                      <span className="absolute right-4 top-1/2 -translate-y-1/2 text-text-muted">%</span>
                    )}
                  </div>
                </div>
              </div>
            </div>

            {/* Periode Berlaku */}
            <div className="pt-6 border-t border-border-main">
              <h3 className="text-sm font-semibold text-text-main mb-4 flex items-center gap-2">
                <Calendar size={16} className="text-primary" />
                Berlaku untuk jadwal main
              </h3>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div className="space-y-2">
                  <label className="text-sm font-medium text-text-main">Tanggal Mulai</label>
                  <input
                    type="datetime-local"
                    required
                    className="w-full bg-white border border-border-main rounded-xl px-4 py-3 text-text-main focus:outline-none focus:ring-2 focus:ring-primary/20 focus:border-primary"
                    value={formData.starts_at}
                    onChange={e => setFormData(prev => ({ ...prev, starts_at: e.target.value }))}
                  />
                </div>

                <div className="space-y-2">
                  <label className="text-sm font-medium text-text-main">Tanggal Selesai</label>
                  <input
                    type="datetime-local"
                    required
                    className="w-full bg-white border border-border-main rounded-xl px-4 py-3 text-text-main focus:outline-none focus:ring-2 focus:ring-primary/20 focus:border-primary"
                    value={formData.ends_at}
                    onChange={e => setFormData(prev => ({ ...prev, ends_at: e.target.value }))}
                  />
                </div>
              </div>
            </div>

            {/* Status (Only for edit) */}
            {promo && (
              <div className="pt-6 border-t border-border-main">
                <div className="flex items-center justify-between">
                  <div>
                    <h3 className="text-sm font-semibold text-text-main">Status Promo</h3>
                    <p className="text-xs text-text-muted mt-1">Nonaktifkan promo jika sudah tidak digunakan</p>
                  </div>
                  <label className="relative inline-flex items-center cursor-pointer">
                    <input
                      type="checkbox"
                      className="sr-only peer"
                      checked={formData.status === 'ACTIVE'}
                      onChange={(e) => setFormData(prev => ({ ...prev, status: e.target.checked ? 'ACTIVE' : 'INACTIVE' }))}
                    />
                    <div className="w-11 h-6 bg-slate-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-primary"></div>
                  </label>
                </div>
              </div>
            )}
          </form>
        </div>

        <div className="p-6 border-t border-border-main bg-white rounded-b-2xl flex justify-end gap-3 shrink-0">
          <button
            type="button"
            onClick={onClose}
            className="px-6 py-2.5 rounded-xl font-medium text-text-main hover:bg-slate-100 transition-colors"
          >
            Batal
          </button>
          <button
            type="submit"
            form="promoForm"
            disabled={loading}
            className="bg-primary hover:bg-primary/90 disabled:bg-primary/50 text-white px-8 py-2.5 rounded-xl font-medium transition-colors flex items-center"
          >
            {loading ? (
              <span className="flex items-center gap-2">
                <div className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                Menyimpan...
              </span>
            ) : 'Simpan Promo'}
          </button>
        </div>
      </div>
    </div>
  );
}
