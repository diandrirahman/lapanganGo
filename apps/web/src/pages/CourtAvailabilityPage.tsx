import React, { useEffect, useState } from 'react';
import { useParams, useNavigate, useLocation } from 'react-router-dom';
import toast from 'react-hot-toast';
import { PageShell } from '../components/layout/PageShell';
import { LoadingState } from '../components/feedback/LoadingState';
import { ErrorState } from '../components/feedback/ErrorState';
import { fetchCourtAvailability, createBooking, fetchVenueById } from '../lib/api';
import type { AvailabilitySlot } from '../types/booking';
import type { VenueDetail } from '../types/venue';
import { Calendar, Clock, MapPin, CheckCircle2, AlertCircle } from 'lucide-react';
import { useAuth } from '../contexts/AuthContext';

export const CourtAvailabilityPage: React.FC = () => {
  const { venueId, courtId, id } = useParams<{ venueId?: string; courtId?: string; id?: string }>();
  const activeCourtId = courtId || id;
  const navigate = useNavigate();
  const location = useLocation();

  const today = new Date().toISOString().split('T')[0];
  const [selectedDate, setSelectedDate] = useState<string>(today);
  const [slots, setSlots] = useState<AvailabilitySlot[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  
  const [selectedTime, setSelectedTime] = useState<string | null>(null);
  const [isBooking, setIsBooking] = useState(false);
  
  const { token, isAuthenticated, user } = useAuth();

  // Booking Summary Data
  const [venueName, setVenueName] = useState<string>(location.state?.venue?.name || '');
  const [venueAddress, setVenueAddress] = useState<string>(location.state?.venue?.address || '');
  const [courtName, setCourtName] = useState<string>(location.state?.court?.name || '');
  const [pricePerHour, setPricePerHour] = useState<number | null>(location.state?.court?.price_per_hour || null);

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
        setSelectedTime(null);
        
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

  const handleDateChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setSelectedDate(e.target.value);
  };

  const handleSlotClick = (slot: AvailabilitySlot) => {
    if (slot.status !== 'AVAILABLE') return;
    setSelectedTime(slot.start_at);
  };

  const handleCreateBooking = async () => {
    if (!activeCourtId || !selectedTime || !selectedDate) return;
    
    if (!isAuthenticated || !token) {
      navigate('/login');
      return;
    }

    if (user?.role === 'OWNER') {
      toast.error('Gunakan akun customer untuk membuat booking.');
      return;
    }

    try {
      setIsBooking(true);
      setError(null);
      
      const selectedSlot = slots.find(s => s.start_at === selectedTime);
      if (!selectedSlot) return;

      const booking = await createBooking({
        court_id: activeCourtId,
        booking_date: selectedDate,
        start_time: formatTime(selectedSlot.start_at),
        end_time: formatTime(selectedSlot.end_at)
      }, token);

      toast.success('Pemesanan lapangan berhasil!');
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

  const selectedSlotData = slots.find(s => s.start_at === selectedTime);
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
                    const isSelected = selectedTime === slot.start_at;
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
                      {selectedTime && selectedSlotData 
                        ? `${formatTime(selectedSlotData.start_at)} - ${formatTime(selectedSlotData.end_at)}`
                        : '-'}
                    </p>
                  </div>
                </div>

                <div className="pt-4 border-t border-border-main flex justify-between items-center">
                  <p className="text-sm font-bold text-text-muted">Total Pembayaran</p>
                  {pricePerHour !== null && selectedTime ? (
                    <p className="text-lg font-extrabold text-primary">
                      Rp {pricePerHour.toLocaleString('id-ID')}
                    </p>
                  ) : (
                    <p className="text-sm font-bold text-text-main text-right italic">
                      Dihitung setelah<br/>konfirmasi
                    </p>
                  )}
                </div>
              </div>

              <button 
                disabled={!selectedTime || isBooking || !isCustomerAccount}
                onClick={handleCreateBooking}
                className={`w-full py-4 rounded-xl font-extrabold text-white transition-all duration-300 ${
                  selectedTime && !isBooking && isCustomerAccount
                    ? 'bg-primary hover:bg-primary/90' 
                    : 'bg-gray-300 cursor-not-allowed'
                }`}
              >
                {isBooking ? 'Memproses...' : isCustomerAccount ? 'Lanjutkan Pesanan' : 'Khusus Customer'}
              </button>
            </div>
          </div>
        </div>
      </div>
    </PageShell>
  );
};
