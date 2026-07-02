import React, { useEffect, useState, useCallback } from 'react';
import toast from 'react-hot-toast';
import { PageShell } from '../../components/layout/PageShell';
import { useAuth } from '../../contexts/AuthContext';
import { useParams, useSearchParams } from 'react-router-dom';
import { fetchOwnerVenueBookings, verifyPayment } from '../../lib/api';
import type { OwnerBooking } from '../../types/booking';
import { Calendar, Clock, MapPin, CheckCircle, XCircle, AlertCircle } from 'lucide-react';
import { LoadingState } from '../../components/feedback/LoadingState';
import { ErrorState } from '../../components/feedback/ErrorState';
import { ConfirmModal } from '../../components/ui/ConfirmModal';
import { formatRupiah, formatDate } from '../../lib/utils';
import { Pagination } from '../../components/ui/Pagination';

export const OwnerVenueBookingsPage: React.FC = () => {
  const { token } = useAuth();
  const { id: venueId } = useParams<{ id: string }>();
  const [bookings, setBookings] = useState<OwnerBooking[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [searchParams, setSearchParams] = useSearchParams();
  const page = parseInt(searchParams.get('page') || '1', 10);
  const filterDate = searchParams.get('filterDate') || '';
  const filterStatus = searchParams.get('filterStatus') || '';
  const scope = searchParams.get('scope') || undefined;
  const [totalPages, setTotalPages] = useState(1);

  const updateParams = (updates: Record<string, string | null>) => {
    setSearchParams(prevParams => {
      const newParams = new URLSearchParams(prevParams);
      for (const [key, value] of Object.entries(updates)) {
        if (value === null || value === '') {
          newParams.delete(key);
        } else {
          newParams.set(key, value);
        }
      }
      return newParams;
    }, { replace: true });
  };

  const setPage = (p: number) => updateParams({ page: p.toString() });

  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [modalState, setModalState] = useState<{ type: 'verify' | 'reject' | null, bookingId: string | null, isOpen: boolean, error?: string }>({ type: null, bookingId: null, isOpen: false });

  const loadBookings = useCallback(() => {
    if (token && venueId) {
      setIsLoading(true);
      fetchOwnerVenueBookings(venueId, token, filterDate, filterStatus, scope, page, 10)
        .then(data => {
          setBookings(data.data || []);
          setTotalPages(data.total_pages || 1);
        })
        .catch((err) => setError(err.message))
        .finally(() => setIsLoading(false));
    } else {
      setIsLoading(false);
    }
  }, [token, venueId, filterDate, filterStatus, scope, page]);

  useEffect(() => {
    loadBookings();
  }, [loadBookings]);

  if (isLoading) {
    return <PageShell><div className="pt-32 text-center text-text-muted">Memuat...</div></PageShell>;
  }

  const handleVerify = async (isApproved: boolean) => {
    if (!token || !modalState.bookingId) return;
    try {
      setActionLoading('verify');
      await verifyPayment(modalState.bookingId, isApproved, token);
      toast.success(isApproved ? 'Pembayaran berhasil diverifikasi!' : 'Pembayaran ditolak.');
      loadBookings();
      setModalState({ type: null, bookingId: null, isOpen: false });
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Gagal memverifikasi pembayaran';
      setModalState(prev => ({ ...prev, error: msg }));
      toast.error(msg);
    } finally {
      setActionLoading(null);
    }
  };

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'PENDING_PAYMENT':
        return <span className="px-3 py-1 bg-yellow-100 text-yellow-800 text-xs font-bold rounded-full flex items-center gap-1"><AlertCircle className="w-3 h-3" /> Menunggu Pembayaran</span>;
      case 'WAITING_VERIFICATION':
        return <span className="px-3 py-1 bg-yellow-400 text-yellow-900 text-xs font-extrabold rounded-full flex items-center gap-1 border border-yellow-500 animate-pulse shadow-sm"><AlertCircle className="w-3 h-3" /> Menunggu Verifikasi</span>;
      case 'CONFIRMED':
        return <span className="px-3 py-1 bg-blue-100 text-blue-800 text-xs font-bold rounded-full flex items-center gap-1"><CheckCircle className="w-3 h-3" /> Dikonfirmasi</span>;
      case 'PAID':
        return <span className="px-3 py-1 bg-green-100 text-green-800 text-xs font-bold rounded-full flex items-center gap-1"><CheckCircle className="w-3 h-3" /> Lunas</span>;
      case 'COMPLETED':
        return <span className="px-3 py-1 bg-gray-200 text-gray-800 text-xs font-bold rounded-full flex items-center gap-1"><CheckCircle className="w-3 h-3" /> Selesai</span>;
      case 'CANCELLED':
        return <span className="px-3 py-1 bg-red-100 text-red-800 text-xs font-bold rounded-full flex items-center gap-1"><XCircle className="w-3 h-3" /> Dibatalkan</span>;
      default:
        return <span className="px-3 py-1 bg-gray-100 text-gray-800 text-xs font-bold rounded-full">{status}</span>;
    }
  };

  return (
    <PageShell>
      <div className="pt-24 pb-40 max-w-4xl mx-auto px-6">
        <h1 className="text-3xl font-extrabold text-text-main mb-2">Daftar Pesanan Masuk</h1>
        <p className="text-text-muted mb-6">Kelola dan pantau seluruh pesanan di venue ini.</p>

        {scope === 'upcoming' && (
          <div className="mb-8 p-4 bg-primary/10 border border-primary/20 rounded-xl flex items-center gap-3">
            <Calendar className="w-5 h-5 text-primary" />
            <p className="text-sm font-bold text-primary">Menampilkan Pesanan Mendatang</p>
          </div>
        )}

        <div className="bg-white p-4 rounded-2xl border border-border-main mb-8 shadow-sm flex flex-col sm:flex-row gap-4 items-end">
          <div className="flex-1 w-full">
            <label className="block text-sm font-bold text-text-main mb-2">Tanggal</label>
            <input
              type="date"
              className="w-full px-4 py-2.5 rounded-xl border border-border-main focus:ring-2 focus:ring-primary focus:border-primary transition-all outline-none"
              value={filterDate}
              onChange={(e) => {
                updateParams({ filterDate: e.target.value, page: '1' });
              }}
            />
          </div>
          <div className="flex-1 w-full">
            <label className="block text-sm font-bold text-text-main mb-2">Status</label>
            <select
              className="w-full px-4 py-2.5 rounded-xl border border-border-main focus:ring-2 focus:ring-primary focus:border-primary transition-all outline-none bg-white"
              value={filterStatus}
              onChange={(e) => {
                updateParams({ filterStatus: e.target.value, page: '1' });
              }}
            >
              <option value="">Semua Status</option>
              <option value="PENDING_PAYMENT">Menunggu Pembayaran</option>
              <option value="WAITING_VERIFICATION">Menunggu Verifikasi</option>
              <option value="PAID">Lunas</option>
              <option value="COMPLETED">Selesai</option>
              <option value="CANCELLED">Dibatalkan</option>
            </select>
          </div>
        </div>

        {isLoading ? (
          <LoadingState message="Memuat pesanan..." className="bg-white rounded-3xl" />
        ) : error ? (
          <ErrorState message={error} onRetry={() => window.location.reload()} />
        ) : bookings.length === 0 ? (
          <div className="bg-white rounded-3xl p-12 border border-border-main shadow-sm text-center">
            <div className="w-16 h-16 bg-gray-100 text-gray-400 rounded-full flex items-center justify-center mx-auto mb-4">
              <Calendar className="w-8 h-8" />
            </div>
            <h2 className="text-xl font-bold text-text-main mb-2">Belum Ada Pesanan</h2>
            <p className="text-text-muted">Venue ini belum memiliki pesanan masuk.</p>
          </div>
        ) : (
          <>
            <div className="space-y-6">
              {bookings.map((booking) => {
                const courtLabel = booking.court.name;

                return (
                  <div key={booking.id} className="bg-white rounded-2xl p-6 md:p-8 border border-border-main shadow-sm hover:shadow-md transition-shadow">
                    <div className="flex flex-col md:flex-row md:items-start justify-between gap-4 mb-6 pb-6 border-b border-border-main">
                      <div>
                        <div className="flex items-center gap-2 mb-2">
                          {getStatusBadge(booking.status)}
                          <span className="text-xs text-text-muted font-medium">ID: {booking.id.substring(0, 8).toUpperCase()}</span>
                        </div>
                        <h2 className="text-xl font-extrabold text-text-main mb-1">{booking.customer.name}</h2>
                        <p className="text-sm text-text-muted mb-2">
                          {booking.customer.email} | ID: {booking.customer.id.substring(0, 8)}
                        </p>
                        <div className="flex items-center gap-1.5 text-text-muted text-sm font-medium">
                          <MapPin className="w-4 h-4 text-primary" />
                          <span>{booking.venue.name} - {courtLabel}</span>
                        </div>
                      </div>
                      <div className="text-left md:text-right">
                        <p className="text-sm font-bold text-text-muted mb-1">Total Pendapatan</p>
                        <p className="text-2xl font-extrabold text-primary">{formatRupiah(booking.total_price)}</p>
                      </div>
                    </div>

                    <div className="flex flex-col md:flex-row justify-between items-start gap-6">
                      <div className="flex gap-6">
                        <div>
                          <p className="text-xs font-bold text-text-muted mb-1 flex items-center gap-1"><Calendar className="w-3.5 h-3.5" /> Tanggal Main</p>
                          <p className="text-[15px] font-bold text-text-main">{formatDate(booking.booking_date)}</p>
                        </div>
                        <div>
                          <p className="text-xs font-bold text-text-muted mb-1 flex items-center gap-1"><Clock className="w-3.5 h-3.5" /> Waktu</p>
                          <p className="text-[15px] font-bold text-text-main">{booking.start_time} - {booking.end_time}</p>
                        </div>
                      </div>

                      {booking.status === 'WAITING_VERIFICATION' && (
                        <div className="w-full md:w-auto p-4 bg-background-base rounded-2xl border border-border-main">
                          <p className="text-xs font-bold text-text-muted mb-1 flex items-center gap-1">Referensi Pembayaran</p>
                          <p className="text-sm font-bold text-text-main mb-3">{booking.payment_reference || '-'}</p>
                          <div className="flex gap-2">
                            <button
                              onClick={() => setModalState({ type: 'verify', bookingId: booking.id, isOpen: true })}
                              disabled={actionLoading !== null}
                              className="flex-1 px-4 py-2 bg-primary text-white text-sm font-bold rounded-xl hover:bg-primary/90 transition-colors"
                            >
                              Terima
                            </button>
                            <button
                              onClick={() => setModalState({ type: 'reject', bookingId: booking.id, isOpen: true })}
                              disabled={actionLoading !== null}
                              className="flex-1 px-4 py-2 bg-red-50 text-red-600 text-sm font-bold rounded-xl hover:bg-red-100 transition-colors"
                            >
                              Tolak
                            </button>
                          </div>
                        </div>
                      )}
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

      <ConfirmModal
        isOpen={modalState.isOpen}
        title={modalState.type === 'verify' ? 'Terima Pembayaran?' : 'Tolak Pembayaran?'}
        message={modalState.type === 'verify' ? 'Apakah Anda yakin pembayaran ini valid dan ingin mengonfirmasi pesanan?' : 'Apakah Anda yakin ingin menolak pembayaran ini? Status akan dikembalikan ke Menunggu Pembayaran.'}
        confirmText={modalState.type === 'verify' ? 'Ya, Terima' : 'Ya, Tolak'}
        cancelText="Tutup"
        isDestructive={modalState.type === 'reject'}
        onConfirm={() => handleVerify(modalState.type === 'verify')}
        onCancel={() => setModalState({ type: null, bookingId: null, isOpen: false })}
        isLoading={actionLoading !== null}
      />

      {modalState.error && (
        <ConfirmModal
          isOpen={!!modalState.error}
          title="Gagal Memproses"
          message={modalState.error}
          confirmText="Tutup"
          onConfirm={() => setModalState(prev => ({ ...prev, error: undefined }))}
          onCancel={() => setModalState(prev => ({ ...prev, error: undefined }))}
        />
      )}
    </PageShell>
  );
};
