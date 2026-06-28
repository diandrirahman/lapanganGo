import React, { useState, useEffect, useCallback } from 'react';
import { X, CalendarOff, Trash2, Plus } from 'lucide-react';
import { getBlockedSlots, createBlockedSlot, deleteBlockedSlot } from '../../lib/api';
import { ConfirmModal } from '../ui/ConfirmModal';
import { formatDateTime } from '../../lib/utils';
import type { BlockedSlot } from '../../types/venue';

interface BlockedSlotsModalProps {
  isOpen: boolean;
  onClose: () => void;
  token: string;
  courtId: string;
  courtName: string;
}

export const BlockedSlotsModal: React.FC<BlockedSlotsModalProps> = ({
  isOpen,
  onClose,
  token,
  courtId,
  courtName
}) => {
  const [slots, setSlots] = useState<BlockedSlot[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  
  const [isAdding, setIsAdding] = useState(false);
  const [newSlot, setNewSlot] = useState({
    start_at: '',
    end_at: '',
    reason: ''
  });
  
  const [deleteModal, setDeleteModal] = useState<{ isOpen: boolean, slotId: string | null }>({ isOpen: false, slotId: null });

  const loadSlots = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await getBlockedSlots(courtId, token);
      setSlots(data || []);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Gagal memuat jadwal terblokir';
      setError(msg);
    } finally {
      setIsLoading(false);
    }
  }, [courtId, token]);

  useEffect(() => {
    if (isOpen && courtId && token) {
      loadSlots();
      setIsAdding(false);
      setNewSlot({ start_at: '', end_at: '', reason: '' });
    }
  }, [isOpen, courtId, token, loadSlots]);

  const handleAdd = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token) return;
    try {
      setIsLoading(true);
      setError(null);
      
      const payload = {
        start_at: new Date(newSlot.start_at).toISOString(),
        end_at: new Date(newSlot.end_at).toISOString(),
        reason: newSlot.reason
      };

      await createBlockedSlot(courtId, payload, token);
      await loadSlots();
      setIsAdding(false);
      setNewSlot({ start_at: '', end_at: '', reason: '' });
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Gagal menambah jadwal terblokir';
      setError(msg);
      setIsLoading(false);
    }
  };

  const handleDelete = async () => {
    if (!token || !deleteModal.slotId) return;
    try {
      setIsLoading(true);
      setError(null);
      await deleteBlockedSlot(deleteModal.slotId, token);
      await loadSlots();
      setDeleteModal({ isOpen: false, slotId: null });
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Gagal menghapus jadwal terblokir';
      setError(msg);
      setIsLoading(false);
    }
  };

	if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
      <div className="bg-white rounded-3xl w-full max-w-2xl shadow-xl overflow-hidden flex flex-col max-h-[90vh]">
        <div className="px-6 py-5 border-b border-border-main flex justify-between items-center bg-background-base">
          <div>
            <h2 className="text-xl font-extrabold text-text-main flex items-center gap-2">
              <CalendarOff className="w-5 h-5 text-red-500" /> Maintenance / Blokir
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

          {!isAdding && (
            <button
              onClick={() => setIsAdding(true)}
              className="w-full mb-6 py-3 border-2 border-dashed border-border-main rounded-xl text-primary font-bold hover:bg-gray-50 transition-colors flex items-center justify-center gap-2"
            >
              <Plus className="w-5 h-5" /> Tambah Blokir Jadwal
            </button>
          )}

          {isAdding && (
            <form onSubmit={handleAdd} className="mb-8 p-4 bg-gray-50 border border-border-main rounded-xl space-y-4">
              <h3 className="font-bold text-text-main">Tambah Jadwal Baru</h3>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-bold text-text-main mb-1.5">Waktu Mulai</label>
                  <input
                    type="datetime-local"
                    required
                    value={newSlot.start_at}
                    onChange={(e) => setNewSlot(p => ({...p, start_at: e.target.value}))}
                    className="w-full px-3 py-2 rounded-lg border border-border-main focus:border-primary outline-none"
                  />
                </div>
                <div>
                  <label className="block text-sm font-bold text-text-main mb-1.5">Waktu Selesai</label>
                  <input
                    type="datetime-local"
                    required
                    value={newSlot.end_at}
                    onChange={(e) => setNewSlot(p => ({...p, end_at: e.target.value}))}
                    className="w-full px-3 py-2 rounded-lg border border-border-main focus:border-primary outline-none"
                  />
                </div>
              </div>
              <div>
                <label className="block text-sm font-bold text-text-main mb-1.5">Alasan (Opsional)</label>
                <input
                  type="text"
                  placeholder="Contoh: Perbaikan lantai"
                  value={newSlot.reason}
                  onChange={(e) => setNewSlot(p => ({...p, reason: e.target.value}))}
                  className="w-full px-3 py-2 rounded-lg border border-border-main focus:border-primary outline-none"
                />
              </div>
              <div className="flex justify-end gap-2 pt-2">
                <button type="button" onClick={() => setIsAdding(false)} className="px-4 py-2 font-bold text-text-muted hover:text-text-main">Batal</button>
                <button type="submit" disabled={isLoading} className="px-4 py-2 bg-primary text-white font-bold rounded-lg hover:bg-primary/90 disabled:opacity-50">
                  Simpan
                </button>
              </div>
            </form>
          )}

          {isLoading && !isAdding ? (
            <div className="text-center py-10 text-text-muted font-bold">Memuat data...</div>
          ) : slots.length === 0 ? (
            <div className="text-center py-10 border border-border-main rounded-xl">
              <p className="text-text-muted font-medium">Tidak ada jadwal yang sedang diblokir.</p>
            </div>
          ) : (
            <div className="space-y-3">
              {slots.map((slot) => (
                <div key={slot.id} className="p-4 border border-border-main rounded-xl">
                  <div className="flex justify-between items-start">
                    <div>
                      <p className="font-bold text-text-main text-[15px]">{slot.reason}</p>
                      <div className="flex items-center gap-2 text-sm text-text-muted mt-1">
                        <CalendarOff className="w-4 h-4" />
                        <span>{formatDateTime(slot.start_at)} - {formatDateTime(slot.end_at)}</span>
                      </div>
                    </div>
                    <button
                      onClick={() => setDeleteModal({ isOpen: true, slotId: slot.id })}
                      className="p-2 text-red-500 hover:bg-red-50 rounded-lg transition-colors"
                      title="Hapus blokir"
                    >
                      <Trash2 className="w-5 h-5" />
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
      
      <ConfirmModal
        isOpen={deleteModal.isOpen}
        title="Hapus Jadwal Terblokir?"
        message="Jadwal yang sudah dihapus blokirnya akan kembali bisa dipesan oleh pelanggan. Apakah Anda yakin?"
        confirmText="Ya, Hapus"
        cancelText="Batal"
        isDestructive={true}
        onConfirm={handleDelete}
        onCancel={() => setDeleteModal({ isOpen: false, slotId: null })}
        isLoading={isLoading}
      />
    </div>
  );
};
