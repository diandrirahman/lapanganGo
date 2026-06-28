import React, { useState, useEffect } from 'react';
import { X } from 'lucide-react';
import { createOwnerCourt, updateOwnerCourt, fetchSports } from '../../lib/api';

interface CourtModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess: () => void;
  token: string;
  venueId: string;
  court?: any | null; // Pass court object to edit, null to create
}

export const CourtModal: React.FC<CourtModalProps> = ({
  isOpen,
  onClose,
  onSuccess,
  token,
  venueId,
  court
}) => {
  const [formData, setFormData] = useState({
    sport_id: '',
    name: '',
    description: '',
    location_type: 'INDOOR',
    surface_type: '',
    price_per_hour: ''
  });

  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [sports, setSports] = useState<any[]>([]);

  useEffect(() => {
    fetchSports().then(setSports).catch(console.error);
  }, []);

  useEffect(() => {
    if (isOpen) {
      if (court) {
        setFormData({
          sport_id: court.sport.id || '',
          name: court.name || '',
          description: court.description || '',
          location_type: court.location_type || 'INDOOR',
          surface_type: court.surface_type || '',
          price_per_hour: court.price_per_hour?.toString() || ''
        });
      } else {
        setFormData({
          sport_id: '',
          name: '',
          description: '',
          location_type: 'INDOOR',
          surface_type: '',
          price_per_hour: ''
        });
      }
      setError(null);
    }
  }, [isOpen, court]);

  if (!isOpen) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token) return;

    try {
      setIsLoading(true);
      setError(null);
      const payload = {
        ...formData,
        price_per_hour: parseFloat(formData.price_per_hour)
      };

      if (court) {
        await updateOwnerCourt(court.id, payload, token);
      } else {
        await createOwnerCourt(venueId, payload, token);
      }
      onSuccess();
      onClose();
    } catch (err: any) {
      setError(err.message || 'Gagal menyimpan lapangan');
    } finally {
      setIsLoading(false);
    }
  };

  const handleChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>) => {
    const { name, value } = e.target;
    setFormData(prev => ({ ...prev, [name]: value }));
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
      <div className="bg-white rounded-3xl w-full max-w-lg shadow-xl overflow-hidden flex flex-col max-h-[90vh]">
        <div className="px-6 py-5 border-b border-border-main flex justify-between items-center bg-background-base">
          <h2 className="text-xl font-extrabold text-text-main">
            {court ? 'Edit Lapangan' : 'Tambah Lapangan Baru'}
          </h2>
          <button onClick={onClose} className="p-2 text-text-muted hover:text-text-main hover:bg-gray-100 rounded-full transition-colors">
            <X className="w-5 h-5" />
          </button>
        </div>

        <div className="p-6 overflow-y-auto">
          {error && (
            <div className="mb-6 p-4 bg-red-50 text-red-700 rounded-xl text-sm font-bold border border-red-100">
              {error}
            </div>
          )}

          <form id="court-form" onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label className="block text-sm font-bold text-text-main mb-1.5">Olahraga *</label>
              <select
                name="sport_id"
                required
                value={formData.sport_id}
                onChange={handleChange}
                className="w-full px-4 py-3 rounded-xl border border-border-main focus:border-primary focus:ring-1 focus:ring-primary outline-none bg-white"
              >
                <option value="" disabled>Pilih Olahraga</option>
                {sports.map(sport => (
                  <option key={sport.id} value={sport.id}>{sport.name}</option>
                ))}
              </select>
            </div>

            <div>
              <label className="block text-sm font-bold text-text-main mb-1.5">Nama Lapangan *</label>
              <input
                type="text"
                name="name"
                required
                value={formData.name}
                onChange={handleChange}
                placeholder="Misal: Lapangan 1"
                className="w-full px-4 py-3 rounded-xl border border-border-main focus:border-primary focus:ring-1 focus:ring-primary outline-none"
              />
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-bold text-text-main mb-1.5">Tipe Lokasi *</label>
                <select
                  name="location_type"
                  value={formData.location_type}
                  onChange={handleChange}
                  className="w-full px-4 py-3 rounded-xl border border-border-main focus:border-primary focus:ring-1 focus:ring-primary outline-none"
                >
                  <option value="INDOOR">INDOOR</option>
                  <option value="OUTDOOR">OUTDOOR</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-bold text-text-main mb-1.5">Permukaan</label>
                <input
                  type="text"
                  name="surface_type"
                  value={formData.surface_type}
                  onChange={handleChange}
                  placeholder="Misal: Vinyl, Rumput"
                  className="w-full px-4 py-3 rounded-xl border border-border-main focus:border-primary focus:ring-1 focus:ring-primary outline-none"
                />
              </div>
            </div>

            <div>
              <label className="block text-sm font-bold text-text-main mb-1.5">Harga per Jam (Rp) *</label>
              <input
                type="number"
                name="price_per_hour"
                required
                min="0"
                value={formData.price_per_hour}
                onChange={handleChange}
                placeholder="100000"
                className="w-full px-4 py-3 rounded-xl border border-border-main focus:border-primary focus:ring-1 focus:ring-primary outline-none"
              />
            </div>

            <div>
              <label className="block text-sm font-bold text-text-main mb-1.5">Deskripsi Lapangan</label>
              <textarea
                name="description"
                rows={2}
                value={formData.description}
                onChange={handleChange}
                placeholder="Opsional..."
                className="w-full px-4 py-3 rounded-xl border border-border-main focus:border-primary focus:ring-1 focus:ring-primary outline-none"
              />
            </div>
          </form>
        </div>

        <div className="px-6 py-5 border-t border-border-main flex gap-3">
          <button
            type="button"
            onClick={onClose}
            className="flex-1 py-3.5 rounded-xl border-2 border-border-main text-text-main font-bold hover:bg-gray-50 transition-colors"
          >
            Batal
          </button>
          <button
            type="submit"
            form="court-form"
            disabled={isLoading}
            className="flex-1 py-3.5 rounded-xl bg-primary text-white font-bold hover:bg-primary/90 transition-colors disabled:opacity-50"
          >
            {isLoading ? 'Menyimpan...' : 'Simpan Lapangan'}
          </button>
        </div>
      </div>
    </div>
  );
};
