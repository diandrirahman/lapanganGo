import React, { useEffect, useState, useCallback } from 'react';
import { PageShell } from '../components/layout/PageShell';
import { useAuth } from '../contexts/AuthContext';
import { LoadingState } from '../components/feedback/LoadingState';
import { ErrorState } from '../components/feedback/ErrorState';
import { formatRupiah, formatDate } from '../lib/utils';
import { fetchCustomerBookings } from '../lib/api';
import { formatPaymentDeadline, getRemainingPaymentMs } from '../lib/paymentExpiry';
import type { Booking } from '../types/booking';
import { Calendar, Clock, MapPin, CheckCircle, XCircle, AlertCircle, Users } from 'lucide-react';
import { CreateMabarModal } from '../components/CreateMabarModal';
import { useNavigate } from 'react-router-dom';
import { Pagination } from '../components/ui/Pagination';

export const CustomerBookingsPage: React.FC = () => {
  const { token } = useAuth();
  const [bookings, setBookings] = useState<Booking[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedBookingForMabar, setSelectedBookingForMabar] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  
  const navigate = useNavigate();

  const loadBookings = useCallback(async () => {
    if (!token) return;
    try {
      setIsLoading(true);
      setError(null);
      const data = await fetchCustomerBookings(token, page, 10);
      setBookings(data.data || []);
      setTotalPages(data.total_pages || 1);
    } catch (err: any) {
      setError(err.message || 'Gagal memuat daftar pesanan');
    } finally {
      setIsLoading(false);
    }
  }, [token, page]);

  useEffect(() => {
    if (token) {
      loadBookings();
    }
  }, [token, loadBookings]);

	const getStatusBadge = (status: string) => {
    switch (status) {
      case 'PENDING_PAYMENT':
        return <span className="px-3 py-1 bg-yellow-100 text-yellow-800 text-xs font-bold rounded-full flex items-center gap-1"><AlertCircle className="w-3 h-3" /> Menunggu Pembayaran</span>;
      case 'WAITING_VERIFICATION':
        return <span className="px-3 py-1 bg-blue-100 text-blue-800 text-xs font-bold rounded-full flex items-center gap-1"><Clock className="w-3 h-3" /> Menunggu Verifikasi</span>;
      case 'PAID':
        return <span className="px-3 py-1 bg-green-100 text-green-800 text-xs font-bold rounded-full flex items-center gap-1"><CheckCircle className="w-3 h-3" /> Lunas</span>;
      case 'COMPLETED':
        return <span className="px-3 py-1 bg-gray-100 text-gray-800 text-xs font-bold rounded-full flex items-center gap-1"><CheckCircle className="w-3 h-3" /> Selesai</span>;
      case 'CANCELLED':
        return <span className="px-3 py-1 bg-red-100 text-red-800 text-xs font-bold rounded-full flex items-center gap-1"><XCircle className="w-3 h-3" /> Dibatalkan</span>;
      case 'CONFIRMED':
        return <span className="px-3 py-1 bg-blue-100 text-blue-800 text-xs font-bold rounded-full flex items-center gap-1"><CheckCircle className="w-3 h-3" /> Dikonfirmasi</span>;
      default:
        return <span className="px-3 py-1 bg-gray-100 text-gray-800 text-xs font-bold rounded-full">{status}</span>;
    }
  };

  return (
    <PageShell>
      <div className="pt-24 pb-40 max-w-4xl mx-auto px-6">
        <h1 className="text-3xl font-extrabold text-text-main mb-6">Pesanan Saya</h1>
        
        {isLoading ? (
          <LoadingState message="Memuat daftar pesanan..." className="bg-white rounded-3xl p-8 border border-border-main" />
        ) : error ? (
          <ErrorState message={error} onRetry={loadBookings} />
        ) : bookings.length === 0 ? (
          <div className="bg-white rounded-3xl p-12 border border-border-main shadow-sm text-center">
            <div className="w-16 h-16 bg-gray-100 text-gray-400 rounded-full flex items-center justify-center mx-auto mb-4">
              <Calendar className="w-8 h-8" />
            </div>
            <h2 className="text-xl font-bold text-text-main mb-2">Belum Ada Pesanan</h2>
            <p className="text-text-muted mb-6">Anda belum pernah memesan lapangan. Yuk, cari lapangan sekarang!</p>
          </div>
        ) : (
          <>
            <div className="space-y-6">
            {bookings.map((booking) => {
              const courtLabel = booking.court ? booking.court.name : `Lapangan #${booking.court_id.substring(0, 8).toUpperCase()}`;
              const sportLabel = booking.court?.sport_name ? ` (${booking.court.sport_name})` : '';
              const venueLabel = booking.venue ? booking.venue.name : 'Unknown Venue';
              const venueAddress = booking.venue?.address ? `${booking.venue.address}${booking.venue.city ? `, ${booking.venue.city}` : ''}` : '';

              return (
                <div key={booking.id} className="bg-white rounded-3xl p-6 md:p-8 border border-border-main shadow-sm hover:shadow-md transition-shadow">
                  <div className="flex flex-col md:flex-row md:items-start justify-between gap-4 mb-6 pb-6 border-b border-border-main">
                    <div>
                      <div className="flex items-center gap-2 mb-2">
                        {getStatusBadge(booking.status)}
                        <span className="text-xs text-text-muted font-medium">ID: {booking.id.substring(0, 8).toUpperCase()}</span>
                      </div>
                      <h2 className="text-xl font-extrabold text-text-main mb-1">{venueLabel}</h2>
                      {venueAddress && <p className="text-sm text-text-muted mb-3">{venueAddress}</p>}
                      <div className="flex items-center gap-1.5 text-text-muted text-sm font-medium">
                        <MapPin className="w-4 h-4 text-primary" />
                        <span>{courtLabel}{sportLabel}</span>
                      </div>
                      {booking.status === 'PENDING_PAYMENT' && booking.expires_at && (
                        <div className="mt-3 inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-bold bg-orange-50 text-orange-700">
                          <AlertCircle className="w-3.5 h-3.5" />
                          {getRemainingPaymentMs(booking.expires_at) > 0
                            ? `Bayar sebelum ${formatPaymentDeadline(booking.expires_at)}`
                            : 'Batas pembayaran lewat'}
                        </div>
                      )}
                    </div>
                    <div className="text-left md:text-right">
                      <p className="text-sm font-bold text-text-muted mb-1">Total Harga</p>
                      <p className="text-2xl font-extrabold text-primary">{formatRupiah(booking.total_price)}</p>
                    </div>
                  </div>

                  <div className="flex flex-col md:flex-row gap-6 md:items-center justify-between">
                    <div className="flex gap-6">
                      <div>
                        <p className="text-xs font-bold text-text-muted mb-1 flex items-center gap-1"><Calendar className="w-3.5 h-3.5" /> Tanggal</p>
                        <p className="text-[15px] font-bold text-text-main">{formatDate(booking.booking_date)}</p>
                      </div>
                      <div>
                        <p className="text-xs font-bold text-text-muted mb-1 flex items-center gap-1"><Clock className="w-3.5 h-3.5" /> Waktu</p>
                        <p className="text-[15px] font-bold text-text-main">{booking.start_time} - {booking.end_time}</p>
                      </div>
                    </div>

                    <div className="flex flex-wrap gap-3 mt-4 md:mt-0">
                      <button
                        onClick={() => navigate(`/bookings/${booking.id}`)}
                        className="px-5 py-2.5 rounded-xl border-2 border-border-main text-text-main font-bold hover:bg-gray-50 transition-colors text-sm"
                      >
                        Lihat Detail
                      </button>
                      {booking.status === 'PENDING_PAYMENT' && (!booking.expires_at || getRemainingPaymentMs(booking.expires_at) > 0) && (
                        <button 
                          onClick={() => navigate(`/bookings/${booking.id}`)}
                          className="w-full sm:w-auto bg-primary text-white px-4 py-2 rounded-xl text-sm font-bold hover:bg-primary-dark transition-colors flex items-center justify-center gap-2"
                        >
                          Detail Pembayaran
                        </button>
                      )}
                      {(booking.status === 'CONFIRMED' || booking.status === 'PAID') && (
                        <button
                          onClick={() => setSelectedBookingForMabar(booking.id)}
                          className="px-5 py-2.5 rounded-xl bg-secondary text-white font-bold hover:bg-secondary/90 shadow-sm transition-all flex items-center gap-2 text-sm"
                        >
                          <Users className="w-4 h-4" />
                          Buat Mabar
                        </button>
                      )}
                    </div>
                    </div>
                  </div>
                );
              })}
            </div>
            <Pagination
              page={page}
              totalPages={totalPages}
              onPageChange={setPage}
            />
          </>
        )}
      </div>

      {selectedBookingForMabar && (
        <CreateMabarModal
          bookingId={selectedBookingForMabar}
          onClose={() => setSelectedBookingForMabar(null)}
          onSuccess={(mabarId) => {
            setSelectedBookingForMabar(null);
            navigate(`/open-matches/${mabarId}`);
          }}
        />
      )}
    </PageShell>
  );
};
