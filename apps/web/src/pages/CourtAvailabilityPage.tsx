import React, { useEffect, useState, useCallback } from 'react';
import { useParams, useNavigate, useLocation } from 'react-router-dom';
import toast from 'react-hot-toast';
import { PageShell } from '../components/layout/PageShell';
import { LoadingState } from '../components/feedback/LoadingState';
import { ErrorState } from '../components/feedback/ErrorState';
import { fetchCourtAvailability, createBooking, fetchVenueById, validatePromo } from '../lib/api';
import { formatPaymentDeadline } from '../lib/paymentExpiry';
import type { AvailabilitySlot } from '../types/booking';
import type { VenueDetail } from '../types/venue';
import { Calendar, Clock, MapPin, CheckCircle2, AlertCircle } from 'lucide-react';
import { useAuth } from '../contexts/AuthContext';
import { toggleContiguousSlotSelection, getSelectedSlotRange, areSelectedSlotsStillAvailable } from '../lib/slotSelection';
import { getLocalTodayDateString } from '../lib/utils';

export const CourtAvailabilityPage: React.FC = () => {
  const { venueId, courtId, id } = useParams<{ venueId?: string; courtId?: string; id?: string }>();
  const activeCourtId = courtId || id;
  const navigate = useNavigate();
  const location = useLocation();

  const today = getLocalTodayDateString();
  
  const getInitialDate = () => {
    const params = new URLSearchParams(location.search);
    const pd = params.get('play_date');
    if (pd && /^\d{4}-\d{2}-\d{2}$/.test(pd) && pd >= today) {
      return pd;
    }
    return today;
  };

  const [selectedDate, setSelectedDate] = useState<string>(getInitialDate);
  const [slots, setSlots] = useState<AvailabilitySlot[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  
  const [selectedSlots, setSelectedSlots] = useState<AvailabilitySlot[]>([]);
  const [isBooking, setIsBooking] = useState(false);
  
  const { token, isAuthenticated, user } = useAuth();

  // Booking Summary Data
  const [venueName, setVenueName] = useState<string>(location.state?.venue?.name || '');
  const [venueAddress, setVenueAddress] = useState<string>(location.state?.venue?.address || '');
  const [courtName, setCourtName] = useState<string>(location.state?.court?.name || '');
  const [pricePerHour, setPricePerHour] = useState<number | null>(location.state?.court?.price_per_hour || null);
  
  const searchParams = new URLSearchParams(location.search);
  const initialPromo = searchParams.get('promo') || '';
  
  const [promoCode, setPromoCode] = useState(initialPromo);
  const [appliedPromo, setAppliedPromo] = useState<any | null>(null);
  const [isValidatingPromo, setIsValidatingPromo] = useState(false);
  const [promoError, setPromoError] = useState<string | null>(null);

  useEffect(() => {
    // If we don't have venueName from state, but we have venueId, fetch it
    if (!venueName && venueId) {
      fetchVenueById(venueId).then((v: VenueDetail) => {
        setVenueName(v.name);
        setVenueAddress(`${v.address}, ${v.city}`);
        if (activeCourtId) {
          const c = v.courts.find(c => c.id === activeCourtId);
          if (c) {
            setCourtName(c.name);
            setPricePerHour(c.price_per_hour);
          }
        }
      }).catch(console.error);
    }
  }, [venueId, activeCourtId, venueName]);

  const formatTime = (isoString: string) => {
    try {
      const date = new Date(isoString);
      const hours = date.getHours().toString().padStart(2, '0');
      const minutes = date.getMinutes().toString().padStart(2, '0');
      if (isNaN(date.getTime())) throw new Error('Invalid Date');
      return `${hours}:${minutes}`;
    } catch {
      return isoString.substring(0, 5);
    }
  };

  useEffect(() => {
    if (!activeCourtId) return;

    const loadAvailability = async () => {
      try {
        setIsLoading(true);
        setError(null);
        setSelectedSlots([]);
        
        const data = await fetchCourtAvailability(activeCourtId, selectedDate);
        setSlots(data.slots || []);
      } catch (err: any) {
        setError(err.message || 'Gagal memuat ketersediaan jadwal');
      } finally {
        setIsLoading(false);
      }
    };

    loadAvailability();
  }, [activeCourtId, selectedDate]);

  // Auto refresh availability
  const refreshAvailability = useCallback(async (preserveSelection: boolean, currentSelected: AvailabilitySlot[]) => {
    if (!activeCourtId || !selectedDate) return;
    try {
      const data = await fetchCourtAvailability(activeCourtId, selectedDate);
      const latestSlots = data.slots || [];
      setSlots(latestSlots);
      
      if (preserveSelection && currentSelected.length > 0) {
        if (!areSelectedSlotsStillAvailable(currentSelected, latestSlots)) {
           setSelectedSlots([]);
           toast.error('Slot yang dipilih sudah tidak tersedia. Silakan pilih jadwal lain.');
        }
      }
    } catch (err) {
      console.error('Failed to refresh availability', err);
    }
  }, [activeCourtId, selectedDate]);

  useEffect(() => {
    if (!activeCourtId || !selectedDate) return;

    const interval = window.setInterval(() => {
      if (document.visibilityState !== 'visible') return;
      refreshAvailability(true, selectedSlots);
    }, 30_000);

    return () => window.clearInterval(interval);
  }, [activeCourtId, selectedDate, refreshAvailability, selectedSlots]);

  useEffect(() => {
    const onVisibilityChange = () => {
      if (document.visibilityState === 'visible') {
        refreshAvailability(true, selectedSlots);
      }
    };

    document.addEventListener('visibilitychange', onVisibilityChange);
    return () => document.removeEventListener('visibilitychange', onVisibilityChange);
  }, [refreshAvailability, selectedSlots]);

  useEffect(() => {
    setAppliedPromo(null);
    setPromoError(null);
  }, [selectedDate, selectedSlots, activeCourtId]);

  const handleDateChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setSelectedDate(e.target.value);
  };

  const handleSlotClick = (slot: AvailabilitySlot) => {
    if (slot.status !== 'AVAILABLE') return;
    if (new Date(slot.start_at) < new Date()) return;

    setSelectedSlots(prev => {
      const result = toggleContiguousSlotSelection(prev, slot);
      if (result.resetHappened && prev.length > 1) {
         toast('Pilih slot berurutan', { icon: '💡' });
      }
      return result.selection;
    });
  };

  const getPromoErrorMessage = (message: string): string => {
    const normalized = message.toLowerCase();
    if (normalized.includes('invalid request payload')) return 'Kode promo tidak valid.';
    if (normalized.includes('promo not found')) return 'Kode promo tidak ditemukan.';
    if (normalized.includes('not active')) return 'Promo sedang tidak aktif.';
    if (normalized.includes('not started')) return 'Promo belum berlaku untuk tanggal booking ini.';
    if (normalized.includes('expired')) return 'Promo sudah berakhir untuk tanggal booking ini.';
    if (normalized.includes('not valid for this venue') || normalized.includes('venue mismatch')) return 'Promo tidak berlaku untuk venue ini.';
    return 'Promo tidak dapat digunakan.';
  };

  const handleApplyPromo = async () => {
    if (!promoCode.trim() || !token) return;

    if (promoCode.trim().length < 3) {
      setPromoError('Kode promo minimal 3 karakter.');
      return;
    }

    const selectedRange = getSelectedSlotRange(selectedSlots);
    if (!selectedRange || !activeCourtId) return;

    setIsValidatingPromo(true);
    setPromoError(null);
    try {
      const res = await validatePromo({
        venue_id: location.state?.venue?.id || venueId || '',
        court_id: activeCourtId,
        booking_date: selectedDate,
        start_time: selectedRange.startTime,
        end_time: selectedRange.endTime,
        promo_code: promoCode.trim()
      }, token);
      setAppliedPromo(res);
      toast.success(`Promo ${res.promo_code} berhasil diterapkan!`);
    } catch (err: any) {
      setAppliedPromo(null);
      const msg = getPromoErrorMessage(err.message || '');
      setPromoError(msg);
      toast.error(msg);
    } finally {
      setIsValidatingPromo(false);
    }
  };

  const handleCreateBooking = async () => {
    if (!activeCourtId || selectedSlots.length === 0 || !selectedDate) return;
    
    if (!isAuthenticated || !token) {
      navigate('/login');
      return;
    }

    if (user?.role === 'OWNER' || user?.role === 'STAFF') {
      toast.error('Gunakan akun customer untuk membuat booking.');
      return;
    }

    try {
      setIsBooking(true);
      setError(null);
      
      const selectedRange = getSelectedSlotRange(selectedSlots);
      if (!selectedRange) {
        setIsBooking(false);
        return;
      }

      const latestAvailability = await fetchCourtAvailability(activeCourtId, selectedDate);
      const latestSlots = latestAvailability.slots || [];

      if (!areSelectedSlotsStillAvailable(selectedSlots, latestSlots)) {
        setSlots(latestSlots);
        setSelectedSlots([]);
        toast.error('Slot yang dipilih sudah tidak tersedia. Silakan pilih jadwal lain.');
        setIsBooking(false);
        return;
      }

      const payload: any = {
        court_id: activeCourtId,
        booking_date: selectedDate,
        start_time: selectedRange.startTime,
        end_time: selectedRange.endTime
      };
      
      if (appliedPromo) {
        payload.promo_code = appliedPromo.promo_code;
      }

      const booking = await createBooking(payload, token);

      if (booking.expires_at) {
        toast.success(`Pesanan dibuat. Selesaikan pembayaran sebelum ${formatPaymentDeadline(booking.expires_at)}.`);
      } else {
        toast.success('Pesanan dibuat. Selesaikan pembayaran dalam 30 menit.');
      }
      navigate(`/bookings/${booking.id}`);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Gagal membuat pesanan. Silakan coba lagi.';
      setError(msg);
      toast.error(msg);
    } finally {
      setIsBooking(false);
    }
  };

  const getSlotStyle = (status: 'AVAILABLE' | 'BOOKED' | 'BLOCKED', isSelected: boolean, isPast: boolean) => {
    if (isSelected) {
      return 'bg-primary text-white border-primary ring-2 ring-primary/30';
    }
    if (isPast && status === 'AVAILABLE') {
      return 'bg-gray-50 text-gray-400 border-gray-200 cursor-not-allowed opacity-60';
    }
    switch (status) {
      case 'AVAILABLE':
        return 'bg-white text-text-main border-border-main hover:border-primary hover:text-primary cursor-pointer';
      case 'BOOKED':
        return 'bg-gray-100 text-gray-400 border-gray-200 cursor-not-allowed opacity-60';
      case 'BLOCKED':
        return 'bg-red-50 text-red-300 border-red-100 cursor-not-allowed opacity-60';
      default:
        return 'bg-white border-border-main';
    }
  };

  const selectedRange = getSelectedSlotRange(selectedSlots);
  const selectedSlotCount = selectedSlots.length;
  const totalPrice = pricePerHour ? pricePerHour * selectedSlotCount : null;
  const isCustomerAccount = !isAuthenticated || user?.role === 'CUSTOMER';

  return (
    <PageShell>
      <div className="pt-24 pb-40 max-w-6xl mx-auto px-6">
        <div className="mb-8">
          <button 
            onClick={() => navigate(-1)}
            className="text-text-muted hover:text-primary mb-4 font-bold flex items-center gap-1 transition-colors text-sm"
          >
            &larr; Kembali
          </button>
          <h1 className="text-3xl md:text-4xl font-extrabold text-text-main mb-2">Pilih Jadwal</h1>
          <p className="text-lg text-text-muted font-medium">Tentukan tanggal dan jam untuk penyewaan lapangan.</p>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
          {/* Left Column: Date & Time Selection */}
          <div className="lg:col-span-2 space-y-8">
            {/* Date Picker */}
            <div className="bg-white rounded-2xl p-6 border border-border-main shadow-sm flex flex-col sm:flex-row gap-4 sm:items-center justify-between">
              <div>
                <label className="block text-sm font-bold text-text-main mb-2 flex items-center gap-2">
                  <Calendar className="w-4 h-4" />
                  Pilih Tanggal
                </label>
                <input 
                  type="date" 
                  value={selectedDate}
                  min={today}
                  onChange={handleDateChange}
                  className="w-full sm:w-auto border border-border-main rounded-xl px-4 py-2.5 font-medium text-text-main focus:outline-none focus:ring-2 focus:ring-primary/20 focus:border-primary transition-all"
                />
              </div>
            </div>

            {/* Slots Content */}
            {isLoading ? (
              <div className="py-20 bg-white rounded-2xl border border-border-main shadow-sm">
                <LoadingState message="Memeriksa jadwal..." />
              </div>
            ) : error ? (
              <div className="py-20 bg-white rounded-2xl border border-border-main shadow-sm">
                <ErrorState message={error} onRetry={() => window.location.reload()} />
              </div>
            ) : (
              <div className="bg-white rounded-2xl p-6 md:p-8 border border-border-main shadow-sm">
                
                {!isCustomerAccount && (
                  <div className="mb-6 p-4 bg-blue-50 border border-blue-200 rounded-xl text-blue-800 text-sm">
                    <p className="font-bold flex items-center gap-2 mb-1">
                      <AlertCircle className="w-4 h-4" />
                      Akses Terbatas
                    </p>
                    <p>Booking hanya tersedia untuk akun customer. Gunakan akun customer untuk membuat booking.</p>
                  </div>
                )}

                {/* Legend */}
                <div className="mb-4">
                  <p className="text-sm text-text-muted italic">Pilih satu atau beberapa slot berurutan.</p>
                </div>
                <div className="flex items-center gap-6 mb-6 pb-6 border-b border-border-main flex-wrap">
                  <div className="flex items-center gap-2">
                    <div className="w-3 h-3 rounded-full bg-white border border-border-main"></div>
                    <span className="text-sm font-bold text-text-main">Tersedia</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <div className="w-3 h-3 rounded-full bg-primary border border-primary"></div>
                    <span className="text-sm font-bold text-text-main">Terpilih</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <div className="w-3 h-3 rounded-full bg-gray-100 border border-gray-200"></div>
                    <span className="text-sm font-bold text-text-muted">Dipesan</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <div className="w-3 h-3 rounded-full bg-red-50 border border-red-100"></div>
                    <span className="text-sm font-bold text-text-muted">Perbaikan</span>
                  </div>
                </div>

                {/* Grid */}
                <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-3 mb-4">
                  {slots.map(slot => {
                    const isSelected = selectedSlots.some(s => s.start_at === slot.start_at);
                    const isPast = new Date(slot.start_at) < new Date();
                    const isDisabled = slot.status !== 'AVAILABLE' || isPast;

                    return (
                      <button
                        key={slot.start_at}
                        disabled={isDisabled}
                        onClick={() => handleSlotClick(slot)}
                        className={`flex flex-col items-center justify-center p-3 rounded-xl border transition-all duration-200 ${getSlotStyle(slot.status, isSelected, isPast)}`}
                      >
                        <div className="flex items-center gap-1.5">
                          <Clock className="w-4 h-4" />
                          <span className="text-sm font-extrabold">{formatTime(slot.start_at)}</span>
                        </div>
                      </button>
                    );
                  })}
                </div>
              </div>
            )}
          </div>

          {/* Right Column: Booking Summary */}
          <div className="lg:col-span-1">
            <div className="bg-white rounded-2xl p-6 border border-border-main shadow-sm sticky top-24">
              <h2 className="text-xl font-extrabold text-text-main mb-6 flex items-center gap-2">
                <CheckCircle2 className="w-5 h-5 text-primary" />
                Ringkasan Booking
              </h2>

              <div className="space-y-4 mb-6">
                <div>
                  <p className="text-sm font-bold text-text-muted mb-1">Venue</p>
                  <p className="font-bold text-text-main">{venueName || '-'}</p>
                  {venueAddress && (
                    <div className="flex items-start gap-1 mt-1 text-sm text-text-muted">
                      <MapPin className="w-4 h-4 shrink-0 mt-0.5" />
                      <span>{venueAddress}</span>
                    </div>
                  )}
                </div>

                <div className="pt-4 border-t border-border-main">
                  <p className="text-sm font-bold text-text-muted mb-1">Lapangan</p>
                  <p className="font-bold text-text-main">{courtName || '-'}</p>
                </div>

                <div className="pt-4 border-t border-border-main flex justify-between items-center">
                  <div>
                    <p className="text-sm font-bold text-text-muted mb-1">Tanggal</p>
                    <p className="font-bold text-text-main">{selectedDate}</p>
                  </div>
                  <div className="text-right">
                    <p className="text-sm font-bold text-text-muted mb-1">Waktu</p>
                    <p className="font-bold text-text-main">
                      {selectedRange 
                        ? `${selectedRange.startTime} - ${selectedRange.endTime}`
                        : '-'}
                    </p>
                  </div>
                </div>
                <div className="pt-4 border-t border-border-main">
                  <p className="text-sm font-bold text-text-muted mb-2">Kode Promo (Opsional)</p>
                  <div className="flex gap-2">
                    <input
                      type="text"
                      placeholder="Masukkan kode promo"
                      className={`flex-1 rounded-xl border px-4 py-2.5 outline-none uppercase transition-colors ${
                        promoError 
                          ? 'border-red-500 focus:border-red-500 focus:ring-1 focus:ring-red-500' 
                          : appliedPromo
                          ? 'border-emerald-500 bg-emerald-50 text-emerald-700 font-bold focus:border-emerald-500'
                          : 'border-border-main focus:border-primary focus:ring-1 focus:ring-primary'
                      }`}
                      value={promoCode}
                      onChange={(e) => {
                        setPromoCode(e.target.value.toUpperCase());
                        if (appliedPromo || promoError) {
                          setAppliedPromo(null);
                          setPromoError(null);
                        }
                      }}
                      disabled={isValidatingPromo}
                    />
                    <button
                      disabled={!promoCode.trim() || isValidatingPromo || selectedSlotCount === 0 || appliedPromo !== null}
                      onClick={handleApplyPromo}
                      className={`px-4 rounded-xl font-bold transition-all ${
                        !promoCode.trim() || selectedSlotCount === 0 || appliedPromo !== null
                          ? 'bg-gray-100 text-gray-400 cursor-not-allowed'
                          : 'bg-primary text-white hover:bg-primary/90'
                      }`}
                    >
                      {isValidatingPromo ? 'Cek...' : appliedPromo ? 'Aktif' : 'Terapkan'}
                    </button>
                  </div>
                  {promoError && (
                    <p className="text-sm text-red-500 mt-2">{promoError}</p>
                  )}
                  {appliedPromo && (
                    <p className="text-sm text-emerald-600 font-medium mt-2">
                      Berhasil! Diskon Rp {appliedPromo.discount_amount.toLocaleString('id-ID')}
                    </p>
                  )}
                </div>

                <div className="pt-4 border-t border-border-main flex justify-between items-center">
                  <div>
                    <p className="text-sm font-bold text-text-muted">Total Pembayaran</p>
                    <p className="text-xs text-text-muted mt-0.5">{selectedSlotCount} Jam</p>
                  </div>
                  {totalPrice !== null && selectedSlotCount > 0 ? (
                    <div className="text-right">
                      {appliedPromo ? (
                        <>
                          <p className="text-sm text-text-muted line-through">
                            Rp {totalPrice.toLocaleString('id-ID')}
                          </p>
                          <p className="text-lg font-extrabold text-primary">
                            Rp {appliedPromo.final_price.toLocaleString('id-ID')}
                          </p>
                        </>
                      ) : (
                        <p className="text-lg font-extrabold text-primary">
                          Rp {totalPrice.toLocaleString('id-ID')}
                        </p>
                      )}
                    </div>
                  ) : (
                    <p className="text-sm font-bold text-text-main text-right italic">
                      Dihitung setelah<br/>konfirmasi
                    </p>
                  )}
                </div>
              </div>

              <button 
                disabled={selectedSlotCount === 0 || isBooking || !isCustomerAccount}
                onClick={handleCreateBooking}
                className={`w-full py-4 rounded-xl font-extrabold text-white transition-all duration-300 ${
                  selectedSlotCount > 0 && !isBooking && isCustomerAccount
                    ? 'bg-primary hover:bg-primary/90' 
                    : 'bg-gray-300 cursor-not-allowed'
                }`}
              >
                {isBooking ? 'Memproses...' : isCustomerAccount ? 'Lanjutkan Pesanan' : 'Khusus Customer'}
              </button>
              <p className="text-xs text-center text-text-muted mt-4">Jadwal diperbarui otomatis. Slot baru dipesan setelah Anda melanjutkan pesanan.</p>
            </div>
          </div>
        </div>
      </div>
    </PageShell>
  );
};
