import React, { useState, useEffect, useCallback } from 'react';
import { X, Clock } from 'lucide-react';
import { getOperatingHours, updateOperatingHours } from '../../lib/api';

const DAYS = ['Minggu', 'Senin', 'Selasa', 'Rabu', 'Kamis', 'Jumat', 'Sabtu'];

interface OperatingHoursModalProps {
  isOpen: boolean;
  onClose: () => void;
  token: string;
  courtId: string;
  courtName: string;
}

export const OperatingHoursModal: React.FC<OperatingHoursModalProps> = ({
  isOpen,
  onClose,
  token,
  courtId,
  courtName
}) => {
  const [hours, setHours] = useState<any[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadHours = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await getOperatingHours(courtId, token);
      
      // Initialize with 7 days if empty
      const currentHours = data || [];
      const defaultHours = DAYS.map((_, i) => {
        const existing = currentHours.find((h: any) => h.day_of_week === i);
        return existing || { day_of_week: i, open_time: '08:00', close_time: '22:00', is_closed: false };
      });
      setHours(defaultHours);
    } catch (err: any) {
      setError(err.message || 'Gagal memuat jam operasional');
    } finally {
      setIsLoading(false);
    }
  }, [courtId, token]); // DAYS is constant, doesn't need to be in deps if moved outside, but it's inside component so we can just disable the lint line or move it outside. Actually, wait. I will just move DAYS outside the component.

  useEffect(() => {
    if (isOpen && courtId && token) {
      loadHours();
    }
  }, [isOpen, courtId, token, loadHours]);

  const handleSave = async () => {
    if (!token) return;
    try {
      setIsSaving(true);
      setError(null);
      // Ensure format HH:MM
      const formattedHours = hours.map(h => ({
        ...h,
        open_time: h.open_time.length === 5 ? h.open_time : h.open_time.substring(0, 5),
        close_time: h.close_time.length === 5 ? h.close_time : h.close_time.substring(0, 5)
      }));
      await updateOperatingHours(courtId, formattedHours, token);
      onClose();
    } catch (err: any) {
      setError(err.message || 'Gagal menyimpan jam operasional');
    } finally {
      setIsSaving(false);
    }
  };

  const updateHour = (index: number, field: string, value: any) => {
    const newHours = [...hours];
    newHours[index] = { ...newHours[index], [field]: value };
    setHours(newHours);
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
      <div className="bg-white rounded-3xl w-full max-w-2xl shadow-xl overflow-hidden flex flex-col max-h-[90vh]">
        <div className="px-6 py-5 border-b border-border-main flex justify-between items-center bg-background-base">
          <div>
            <h2 className="text-xl font-extrabold text-text-main flex items-center gap-2">
              <Clock className="w-5 h-5 text-primary" /> Jam Operasional
            </h2>
            <p className="text-sm font-medium text-text-muted mt-1">{courtName}</p>
          </div>
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

          {isLoading ? (
            <div className="text-center py-10 text-text-muted font-bold">Memuat data...</div>
          ) : (
            <div className="space-y-4">
              {hours.map((hour, idx) => (
                <div key={idx} className={`flex items-center gap-4 p-4 rounded-xl border ${hour.is_closed ? 'bg-gray-50 border-gray-200 opacity-70' : 'bg-white border-border-main'}`}>
                  <div className="w-24 shrink-0">
                    <span className="font-bold text-text-main">{DAYS[hour.day_of_week]}</span>
                  </div>
                  
                  <div className="flex-1 flex items-center gap-3">
                    <input
                      type="time"
                      disabled={hour.is_closed}
                      value={hour.open_time.substring(0, 5)}
                      onChange={(e) => updateHour(idx, 'open_time', e.target.value)}
                      className="w-full px-3 py-2 rounded-lg border border-border-main focus:border-primary outline-none disabled:bg-gray-100"
                    />
                    <span className="text-text-muted font-bold">-</span>
                    <input
                      type="time"
                      disabled={hour.is_closed}
                      value={hour.close_time.substring(0, 5)}
                      onChange={(e) => updateHour(idx, 'close_time', e.target.value)}
                      className="w-full px-3 py-2 rounded-lg border border-border-main focus:border-primary outline-none disabled:bg-gray-100"
                    />
                  </div>

                  <div className="shrink-0 flex items-center gap-2 ml-4">
                    <label className="text-sm font-bold text-text-muted cursor-pointer flex items-center gap-2">
                      <input
                        type="checkbox"
                        checked={hour.is_closed}
                        onChange={(e) => updateHour(idx, 'is_closed', e.target.checked)}
                        className="w-4 h-4 rounded text-primary focus:ring-primary"
                      />
                      Tutup
                    </label>
                  </div>
                </div>
              ))}
            </div>
          )}
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
            onClick={handleSave}
            disabled={isLoading || isSaving}
            className="flex-1 py-3.5 rounded-xl bg-primary text-white font-bold hover:bg-primary/90 transition-colors disabled:opacity-50"
          >
            {isSaving ? 'Menyimpan...' : 'Simpan Jam Operasional'}
          </button>
        </div>
      </div>
    </div>
  );
};
