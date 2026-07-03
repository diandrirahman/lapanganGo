import React, { useState, useEffect, useCallback } from 'react';
import { X, AlertCircle } from 'lucide-react';
import { fetchOwnerCourtsByVenueId, createOwnerOfflineBooking, fetchCourtAvailability } from '../../lib/api';
import type { Venue, Court } from '../../types/venue';
import type { AvailabilitySlot } from '../../types/booking';
import { toggleContiguousSlotSelection, getSelectedSlotRange, areSelectedSlotsStillAvailable, formatSlotTime } from '../../lib/slotSelection';

interface Props {
  isOpen: boolean;
  onClose: () => void;
  onSuccess: () => void;
  venues: Venue[];
  token: string;
}

export const OwnerOfflineBookingModal: React.FC<Props> = ({ isOpen, onClose, onSuccess, venues, token }) => {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  
  const [venueId, setVenueId] = useState('');
  const [courtId, setCourtId] = useState('');
  const [bookingDate, setBookingDate] = useState('');
  const [slots, setSlots] = useState<AvailabilitySlot[]>([]);
  const [selectedSlots, setSelectedSlots] = useState<AvailabilitySlot[]>([]);
  const [loadingAvailability, setLoadingAvailability] = useState(false);
  const [customerName, setCustomerName] = useState('');
  const [customerPhone, setCustomerPhone] = useState('');
  const [customerEmail, setCustomerEmail] = useState('');
  const [totalPrice, setTotalPrice] = useState<number | ''>('');
  const [status, setStatus] = useState<'PAID' | 'COMPLETED'>('PAID');
  const [note, setNote] = useState('');

  const [courts, setCourts] = useState<Court[]>([]);
  const [loadingCourts, setLoadingCourts] = useState(false);

  useEffect(() => {
    if (!isOpen) {
      // Reset state
      setVenueId('');
      setCourtId('');
      setBookingDate('');
      setSlots([]);
      setSelectedSlots([]);
      setCustomerName('');
      setCustomerPhone('');
      setCustomerEmail('');
      setTotalPrice('');
      setStatus('PAID');
      setNote('');
      setError(null);
      setCourts([]);
    }
  }, [isOpen]);

  useEffect(() => {
    if (venueId && isOpen) {
      setLoadingCourts(true);
      fetchOwnerCourtsByVenueId(venueId, token)
        .then(data => {
          setCourts(data);
          setCourtId('');
        })
        .catch((_err: any) => setCourts([]))
        .finally(() => setLoadingCourts(false));
    } else {
      setCourts([]);
      setCourtId('');
    }
  }, [venueId, token, isOpen]);

  // Fetch availability when court and date are selected
  const refreshAvailability = useCallback(async (preserveSelection: boolean, currentSelected: AvailabilitySlot[]) => {
    if (!courtId || !bookingDate || !isOpen) return;
    try {
      if (!preserveSelection) setLoadingAvailability(true);
      const data = await fetchCourtAvailability(courtId, bookingDate);
      const latestSlots = data.slots || [];
      setSlots(latestSlots);
      
      if (preserveSelection && currentSelected.length > 0) {
        if (!areSelectedSlotsStillAvailable(currentSelected, latestSlots)) {
          setSelectedSlots([]);
          setError('Slot yang dipilih sudah tidak tersedia. Silakan pilih jadwal lain.');
        }
      }
    } catch (err: any) {
      if (!preserveSelection) {
        setSlots([]);
        setError(err.message || 'Gagal memuat jadwal');
      }
    } finally {
      if (!preserveSelection) setLoadingAvailability(false);
    }
  }, [courtId, bookingDate, isOpen]);

  useEffect(() => {
    if (!courtId || !bookingDate || !isOpen) {
      setSlots([]);
      setSelectedSlots([]);
      return;
    }
    
    refreshAvailability(false, []);
  }, [courtId, bookingDate, isOpen, refreshAvailability]);

  useEffect(() => {
    if (!isOpen || !courtId || !bookingDate) return;

    const interval = window.setInterval(() => {
      if (document.visibilityState !== 'visible') return;
      refreshAvailability(true, selectedSlots);
    }, 30_000);

    return () => window.clearInterval(interval);
  }, [isOpen, courtId, bookingDate, refreshAvailability, selectedSlots]);

  useEffect(() => {
    const onVisibilityChange = () => {
      if (document.visibilityState === 'visible' && isOpen && courtId && bookingDate) {
        refreshAvailability(true, selectedSlots);
      }
    };
    document.addEventListener('visibilitychange', onVisibilityChange);
    return () => document.removeEventListener('visibilitychange', onVisibilityChange);
  }, [isOpen, courtId, bookingDate, refreshAvailability, selectedSlots]);

  const handleSlotClick = (slot: AvailabilitySlot) => {
    if (slot.status !== 'AVAILABLE') return;
    if (new Date(slot.start_at) < new Date()) return;

    setSelectedSlots(prev => {
      const result = toggleContiguousSlotSelection(prev, slot);
      if (result.resetHappened && prev.length > 1) {
        setError('Pilih slot berurutan.');
      } else {
        setError(null);
      }
      return result.selection;
    });
  };

  // Automatically compute total price
  useEffect(() => {
    const selectedCourt = courts.find(c => c.id === courtId);
    const price = selectedCourt?.price_per_hour ?? 0;
    if (selectedSlots.length > 0) {
      setTotalPrice(selectedSlots.length * price);
    } else {
      setTotalPrice('');
    }
  }, [selectedSlots, courtId, courts]);

  if (!isOpen) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const selectedRange = getSelectedSlotRange(selectedSlots);
    if (!venueId || !courtId || !bookingDate || !selectedRange || !customerName || totalPrice === '') {
      setError('Harap isi semua field wajib dan pilih jadwal.');
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const latestAvailability = await fetchCourtAvailability(courtId, bookingDate);
      const latestSlots = latestAvailability.slots || [];
      if (!areSelectedSlotsStillAvailable(selectedSlots, latestSlots)) {
        setSlots(latestSlots);
        setSelectedSlots([]);
        setError('Slot yang dipilih sudah tidak tersedia. Silakan pilih jadwal lain.');
        setLoading(false);
        return;
      }

      await createOwnerOfflineBooking(token, {
        venue_id: venueId,
        court_id: courtId,
        booking_date: bookingDate,
        start_time: selectedRange.startTime,
        end_time: selectedRange.endTime,
        customer_name: customerName,
        customer_phone: customerPhone || undefined,
        customer_email: customerEmail || undefined,
        total_price: Number(totalPrice),
        status,
        note: note || undefined,
      });
      onSuccess();
      onClose();
    } catch (err: any) {
      setError(err.message || 'Gagal menambahkan booking offline.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
      <div className="bg-white rounded-xl shadow-xl w-full max-w-2xl max-h-[90vh] overflow-hidden flex flex-col">
        <div className="flex items-center justify-between p-6 border-b border-gray-100">
          <h2 className="text-xl font-semibold text-gray-900">Tambah Booking Offline</h2>
          <button
            onClick={onClose}
            className="p-2 text-gray-400 hover:text-gray-500 hover:bg-gray-100 rounded-full transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        <div className="flex-1 overflow-y-auto p-6">
          {error && (
            <div className="mb-6 p-4 bg-red-50 border border-red-200 rounded-lg flex gap-3 text-red-700">
              <AlertCircle className="w-5 h-5 flex-shrink-0" />
              <p className="text-sm">{error}</p>
            </div>
          )}

          <form id="offlineBookingForm" onSubmit={handleSubmit} className="space-y-6">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
              <div className="space-y-2">
                <label className="text-sm font-medium text-gray-700">Venue *</label>
                <select
                  value={venueId}
                  onChange={e => setVenueId(e.target.value)}
                  required
                  className="w-full rounded-lg border-gray-300 border p-2.5 text-sm focus:ring-emerald-500 focus:border-emerald-500"
                >
                  <option value="">Pilih Venue</option>
                  {venues.map(v => (
                    <option key={v.id} value={v.id}>{v.name}</option>
                  ))}
                </select>
              </div>

              <div className="space-y-2">
                <label className="text-sm font-medium text-gray-700">Lapangan *</label>
                <select
                  value={courtId}
                  onChange={e => setCourtId(e.target.value)}
                  required
                  disabled={!venueId || loadingCourts}
                  className="w-full rounded-lg border-gray-300 border p-2.5 text-sm focus:ring-emerald-500 focus:border-emerald-500 disabled:bg-gray-50 disabled:text-gray-500"
                >
                  <option value="">{loadingCourts ? 'Memuat...' : 'Pilih Lapangan'}</option>
                  {courts.map(c => (
                    <option key={c.id} value={c.id}>{c.name}</option>
                  ))}
                </select>
              </div>

              <div className="space-y-2">
                <label className="text-sm font-medium text-gray-700">Tanggal *</label>
                <input
                  type="date"
                  value={bookingDate}
                  onChange={e => setBookingDate(e.target.value)}
                  required
                  className="w-full rounded-lg border-gray-300 border p-2.5 text-sm focus:ring-emerald-500 focus:border-emerald-500"
                />
              </div>

              <div className="md:col-span-2 space-y-4">
                <div className="flex items-center gap-4 flex-wrap text-sm border-b border-gray-100 pb-2">
                  <div className="flex items-center gap-2">
                    <div className="w-3 h-3 rounded-full bg-white border border-gray-300"></div>
                    <span className="text-gray-600">Tersedia</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <div className="w-3 h-3 rounded-full bg-emerald-600 border border-emerald-600"></div>
                    <span className="text-gray-600">Terpilih</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <div className="w-3 h-3 rounded-full bg-gray-100 border border-gray-200"></div>
                    <span className="text-gray-400">Dipesan</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <div className="w-3 h-3 rounded-full bg-red-50 border border-red-100"></div>
                    <span className="text-gray-400">Perbaikan</span>
                  </div>
                </div>

                {loadingAvailability ? (
                  <div className="py-8 text-center text-gray-500 text-sm">Memuat jadwal...</div>
                ) : !courtId || !bookingDate ? (
                  <div className="py-8 text-center text-gray-500 text-sm">Pilih lapangan dan tanggal untuk melihat jadwal</div>
                ) : slots.length === 0 ? (
                  <div className="py-8 text-center text-gray-500 text-sm">Tidak ada jadwal tersedia</div>
                ) : (
                  <div className="grid grid-cols-3 sm:grid-cols-4 lg:grid-cols-6 gap-2">
                    {slots.map(slot => {
                      const isSelected = selectedSlots.some(s => s.start_at === slot.start_at);
                      const isPast = new Date(slot.start_at) < new Date();
                      const isDisabled = slot.status !== 'AVAILABLE' || isPast;

                      let btnClass = 'bg-white border-gray-200 hover:border-emerald-500 hover:text-emerald-600 text-gray-700 cursor-pointer';
                      if (isSelected) {
                        btnClass = 'bg-emerald-600 text-white border-emerald-600';
                      } else if (slot.status === 'BLOCKED') {
                        btnClass = 'bg-red-50 text-red-300 border-red-100 cursor-not-allowed opacity-60';
                      } else if (isDisabled) {
                        btnClass = 'bg-gray-50 text-gray-400 border-gray-200 cursor-not-allowed opacity-60';
                      }

                      return (
                        <button
                          key={slot.start_at}
                          type="button"
                          disabled={isDisabled}
                          onClick={() => handleSlotClick(slot)}
                          className={`flex items-center justify-center p-2 rounded-lg border text-sm transition-all duration-200 ${btnClass}`}
                        >
                          <span className="font-medium">{formatSlotTime(slot.start_at)}</span>
                        </button>
                      );
                    })}
                  </div>
                )}
                <p className="text-xs text-gray-500">Jadwal diperbarui otomatis setiap 30 detik.</p>
              </div>

              <div className="space-y-2">
                <label className="text-sm font-medium text-gray-700">Nama Customer *</label>
                <input
                  type="text"
                  value={customerName}
                  onChange={e => setCustomerName(e.target.value)}
                  required
                  placeholder="Misal: Budi"
                  className="w-full rounded-lg border-gray-300 border p-2.5 text-sm focus:ring-emerald-500 focus:border-emerald-500"
                />
              </div>

              <div className="space-y-2">
                <label className="text-sm font-medium text-gray-700">No. WhatsApp (Opsional)</label>
                <input
                  type="tel"
                  value={customerPhone}
                  onChange={e => setCustomerPhone(e.target.value)}
                  placeholder="Misal: 08123456789"
                  className="w-full rounded-lg border-gray-300 border p-2.5 text-sm focus:ring-emerald-500 focus:border-emerald-500"
                />
              </div>

              <div className="space-y-2">
                <label className="text-sm font-medium text-gray-700">Email Customer (Opsional)</label>
                <input
                  type="email"
                  value={customerEmail}
                  onChange={e => setCustomerEmail(e.target.value)}
                  placeholder="Misal: budi@email.com"
                  className="w-full rounded-lg border-gray-300 border p-2.5 text-sm focus:ring-emerald-500 focus:border-emerald-500"
                />
              </div>

              <div className="space-y-2">
                <label className="text-sm font-medium text-gray-700">Total Harga (Rp) *</label>
                <input
                  type="number"
                  value={totalPrice}
                  readOnly
                  className="w-full rounded-lg border-gray-300 border p-2.5 text-sm bg-gray-50 text-gray-700"
                />
              </div>

              <div className="space-y-2">
                <label className="text-sm font-medium text-gray-700">Status Pembayaran *</label>
                <select
                  value={status}
                  onChange={e => setStatus(e.target.value as 'PAID' | 'COMPLETED')}
                  required
                  className="w-full rounded-lg border-gray-300 border p-2.5 text-sm focus:ring-emerald-500 focus:border-emerald-500"
                >
                  <option value="PAID">Lunas (PAID)</option>
                  <option value="COMPLETED">Selesai (COMPLETED)</option>
                </select>
              </div>
            </div>

            <div className="space-y-2">
              <label className="text-sm font-medium text-gray-700">Catatan (Opsional)</label>
              <textarea
                value={note}
                onChange={e => setNote(e.target.value)}
                placeholder="Catatan tambahan tentang booking ini"
                rows={3}
                className="w-full rounded-lg border-gray-300 border p-2.5 text-sm focus:ring-emerald-500 focus:border-emerald-500 resize-none"
              />
            </div>
          </form>
        </div>

        <div className="p-6 border-t border-gray-100 flex justify-end gap-3 bg-gray-50 mt-auto">
          <button
            type="button"
            onClick={onClose}
            disabled={loading}
            className="px-5 py-2.5 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-lg hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-emerald-500 disabled:opacity-50"
          >
            Batal
          </button>
          <button
            type="submit"
            form="offlineBookingForm"
            disabled={loading || !venueId || !courtId || !bookingDate || selectedSlots.length === 0 || !customerName || totalPrice === ''}
            className="px-5 py-2.5 text-sm font-medium text-white bg-emerald-600 border border-transparent rounded-lg hover:bg-emerald-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-emerald-500 disabled:opacity-50 flex items-center justify-center gap-2"
          >
            {loading ? (
              <>
                <div className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                <span>Menyimpan...</span>
              </>
            ) : (
              <span>Simpan Booking Offline</span>
            )}
          </button>
        </div>
      </div>
    </div>
  );
};
