import React, { useState } from 'react';
import { createOpenMatch } from '../lib/api';
import { useAuth } from '../contexts/AuthContext';
import { X, Trophy, Users, Wallet, AlignLeft } from 'lucide-react';
import { Button } from './ui/Button';

interface Props {
  bookingId: string;
  onClose: () => void;
  onSuccess: (mabarId: string) => void;
}

export const CreateMabarModal: React.FC<Props> = ({ bookingId, onClose, onSuccess }) => {
  const { token } = useAuth();
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [formData, setFormData] = useState({
    title: '',
    description: '',
    level: 'All Levels',
    max_players: 10 as number | string,
    price_per_player: 0 as number | string
  });

  const handleChange = (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>) => {
    const { name, value } = e.target;
    setFormData(prev => ({
      ...prev,
      [name]: value
    }));
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token) return;

    try {
      setIsLoading(true);
      setError(null);
      
      const payload = {
        ...formData,
        max_players: Number(formData.max_players) || 2,
        price_per_player: Number(formData.price_per_player) || 0
      };
      
      const res = await createOpenMatch(bookingId, payload, token);
      onSuccess(res.id);
    } catch (err: any) {
      setError(err.message || 'Gagal membuat mabar');
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm">
      <div className="bg-white rounded-3xl w-full max-w-lg overflow-hidden shadow-2xl animate-in fade-in zoom-in duration-200">
        <div className="flex justify-between items-center p-6 border-b border-border-main bg-gray-50/50">
          <h2 className="text-xl font-extrabold text-text-main">Buat Jadwal Mabar</h2>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-600 transition-colors p-2 rounded-full hover:bg-gray-100">
            <X className="w-5 h-5" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="p-6">
          {error && (
            <div className="mb-6 p-4 bg-red-50 text-red-600 rounded-xl text-sm font-bold border border-red-100 flex items-start gap-2">
              <span className="shrink-0 mt-0.5">⚠️</span>
              {error}
            </div>
          )}

          <div className="space-y-5">
            <div>
              <label className="block text-sm font-bold text-text-main mb-2">Judul Mabar <span className="text-red-500">*</span></label>
              <input
                type="text"
                name="title"
                required
                minLength={5}
                maxLength={100}
                placeholder="Cth: Fun Football Minggu Pagi"
                className="w-full p-3.5 bg-gray-50 border border-gray-200 rounded-xl focus:ring-2 focus:ring-primary/20 focus:border-primary outline-none transition-all font-medium text-text-main"
                value={formData.title}
                onChange={handleChange}
              />
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-bold text-text-main mb-2 flex items-center gap-1.5"><Users className="w-4 h-4 text-gray-500" /> Kuota Pemain <span className="text-red-500">*</span></label>
                <input
                  type="number"
                  name="max_players"
                  required
                  min={2}
                  max={50}
                  className="w-full p-3.5 bg-gray-50 border border-gray-200 rounded-xl focus:ring-2 focus:ring-primary/20 focus:border-primary outline-none transition-all font-medium text-text-main"
                  value={formData.max_players}
                  onChange={handleChange}
                />
              </div>
              <div>
                <label className="block text-sm font-bold text-text-main mb-2 flex items-center gap-1.5"><Wallet className="w-4 h-4 text-gray-500" /> Patungan (Rp)</label>
                <input
                  type="number"
                  name="price_per_player"
                  min={0}
                  step={1000}
                  className="w-full p-3.5 bg-gray-50 border border-gray-200 rounded-xl focus:ring-2 focus:ring-primary/20 focus:border-primary outline-none transition-all font-medium text-text-main"
                  value={formData.price_per_player === 0 ? '' : formData.price_per_player}
                  onChange={handleChange}
                />
                <p className="text-[11px] text-gray-500 mt-1 font-medium">Kosongkan/0 jika gratis</p>
              </div>
            </div>

            <div>
              <label className="block text-sm font-bold text-text-main mb-2 flex items-center gap-1.5"><Trophy className="w-4 h-4 text-gray-500" /> Level Permainan</label>
              <select
                name="level"
                className="w-full p-3.5 bg-gray-50 border border-gray-200 rounded-xl focus:ring-2 focus:ring-primary/20 focus:border-primary outline-none transition-all font-medium text-text-main"
                value={formData.level}
                onChange={handleChange}
              >
                <option value="Beginner / Fun">Pemula / Fun</option>
                <option value="Intermediate">Menengah</option>
                <option value="Advanced">Mahir</option>
                <option value="All Levels">Semua Level</option>
              </select>
            </div>

            <div>
              <label className="block text-sm font-bold text-text-main mb-2 flex items-center gap-1.5"><AlignLeft className="w-4 h-4 text-gray-500" /> Catatan / Deskripsi</label>
              <textarea
                name="description"
                rows={3}
                placeholder="Cth: Diharapkan bawa bola sendiri, tidak boleh pakai sepatu pul besi."
                className="w-full p-3.5 bg-gray-50 border border-gray-200 rounded-xl focus:ring-2 focus:ring-primary/20 focus:border-primary outline-none transition-all font-medium text-text-main resize-none"
                value={formData.description}
                onChange={handleChange}
              />
            </div>
          </div>

          <div className="mt-8 pt-6 border-t border-border-main flex gap-3">
            <Button type="button" variant="outline" className="flex-1" onClick={onClose}>
              Batal
            </Button>
            <Button type="submit" disabled={isLoading} className="flex-1">
              {isLoading ? 'Memproses...' : 'Buat Mabar Sekarang'}
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
};
