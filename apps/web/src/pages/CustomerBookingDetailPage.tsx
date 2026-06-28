import React, { useEffect, useState, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import toast from 'react-hot-toast';
import { PageShell } from '../components/layout/PageShell';
import { useAuth } from '../contexts/AuthContext';
import { fetchBookingById, cancelBooking, submitPaymentProof } from '../lib/api';
import type { Booking } from '../types/booking';
import { Calendar, Clock, MapPin, CheckCircle, AlertCircle, XCircle, ChevronLeft } from 'lucide-react';
import { LoadingState } from '../components/feedback/LoadingState';
import { ErrorState } from '../components/feedback/ErrorState';
import { ConfirmModal } from '../components/ui/ConfirmModal';
import { formatRupiah, formatDate } from '../lib/utils';

export const CustomerBookingDetailPage: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { token } = useAuth();
  
  const [booking, setBooking] = useState<Booking | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [paymentReference, setPaymentReference] = useState('');
  const [modalState, setModalState] = useState<{ type: 'cancel' | 'pay' | null, isOpen: boolean, error?: string }>({ type: null, isOpen: false });

  const loadBooking = useCallback(async () => {
    if (!token || !id) return;
    try {
      setIsLoading(true);
      setError(null);
      const data = await fetchBookingById(id, token);
      setBooking(data);
    } catch (err: any) {
      setError(err.message || 'Gagal memuat detail pesanan');
    } finally {
      setIsLoading(false);
    }
  }, [id, token]);

  useEffect(() => {
    if (token) {
      loadBooking();
    }
  }, [token, loadBooking]);

  if (isLoading) return <PageShell><LoadingState message="Memuat detail pesanan..." /></PageShell>;

  const handleCancel = async () => {
    if (!token || !id) return;
    try {
      setActionLoading('cancel');
      await cancelBooking(id, token);
      toast.success('Pemesanan berhasil dibatalkan');
      await loadBooking();
      setModalState({ type: null, isOpen: false });
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Gagal membatalkan pesanan';
      setModalState(prev => ({ ...prev, error: msg }));
      toast.error(msg);
    } finally {
      setActionLoading(null);
    }
  };

  const handleConfirmPay = async () => {
    if (!token || !id) return;
    if (!paymentReference.trim()) {
      setModalState(prev => ({ ...prev, error: 'Referensi pembayaran (Nama Pengirim / Bank) harus diisi' }));
      return;
    }
    try {
      setActionLoading('pay');
      await submitPaymentProof(id, paymentReference, token);
      toast.success('Bukti pembayaran berhasil dikirim');
      await loadBooking();
      setModalState({ type: null, isOpen: false });
      setPaymentReference('');
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Gagal mengirim bukti pembayaran';
      setModalState(prev => ({ ...prev, error: msg }));
      toast.error(msg);
    } finally {
      setActionLoading(null);
    }
  };

	const getStatusBadge = (status: string) => {
    switch (status) {
      case 'PENDING_PAYMENT':
        return <span className="px-4 py-1.5 bg-yellow-100 text-yellow-800 text-sm font-bold rounded-full flex items-center gap-1.5 w-fit"><AlertCircle className="w-4 h-4" /> Menunggu Pembayaran</span>;
      case 'WAITING_VERIFICATION':
        return <span className="px-4 py-1.5 bg-blue-100 text-blue-800 text-sm font-bold rounded-full flex items-center gap-1.5 w-fit"><AlertCircle className="w-4 h-4" /> Menunggu Verifikasi</span>;
      case 'CONFIRMED':
        return <span className="px-4 py-1.5 bg-blue-100 text-blue-800 text-sm font-bold rounded-full flex items-center gap-1.5 w-fit"><CheckCircle className="w-4 h-4" /> Dikonfirmasi</span>;
      case 'CANCELLED':
        return <span className="px-4 py-1.5 bg-red-100 text-red-800 text-sm font-bold rounded-full flex items-center gap-1.5 w-fit"><XCircle className="w-4 h-4" /> Dibatalkan</span>;
      default:
        return <span className="px-4 py-1.5 bg-gray-100 text-gray-800 text-sm font-bold rounded-full w-fit">{status}</span>;
    }
  };

  return (
    <PageShell>
      <div className="pt-24 pb-40 max-w-3xl mx-auto px-6">
        <button 
          onClick={() => navigate('/bookings')}
          className="flex items-center gap-2 text-text-muted hover:text-primary transition-colors font-bold mb-8"
        >
          <ChevronLeft className="w-5 h-5" /> Kembali ke Daftar Pesanan
        </button>

        {isLoading ? (
          <LoadingState message="Memuat detail pesanan..." className="bg-white rounded-3xl p-8 border border-border-main" />
        ) : error || !booking ? (
          <ErrorState message={error || 'Pesanan tidak ditemukan'} onRetry={loadBooking} />
        ) : (
          <div className="bg-white rounded-3xl p-8 border border-border-main shadow-sm">
            <div className="flex justify-between items-start mb-8">
              <div>
                <h1 className="text-2xl font-extrabold text-text-main mb-3">Detail Pesanan</h1>
                <p className="text-sm font-medium text-text-muted break-all font-mono">ID: {booking.id}</p>
              </div>
              {getStatusBadge(booking.status)}
            </div>

            <div className="space-y-6">
              {/* Venue & Court Info */}
              <div className="p-4 bg-background-base rounded-2xl">
                <h2 className="text-xl font-bold text-text-main mb-1">
                  {booking.venue?.name || 'Unknown Venue'}
                </h2>
                <div className="flex items-center gap-2 text-text-muted text-sm font-medium mb-4">
                  <MapPin className="w-4 h-4 text-primary" />
                  <span>{booking.venue?.address || 'Unknown Address'}</span>
                </div>
                <div className="text-[15px] font-bold text-text-main">
                  {booking.court?.name || 'Unknown Court'} {booking.court?.sport_name && `(${booking.court.sport_name})`}
                </div>
              </div>

              {/* Date & Time */}
              <div className="grid grid-cols-2 gap-4">
                <div className="p-4 border border-border-main rounded-2xl">
                  <p className="text-sm font-bold text-text-muted mb-1 flex items-center gap-1.5"><Calendar className="w-4 h-4" /> Tanggal</p>
                  <p className="text-[15px] font-bold text-text-main">{formatDate(booking.booking_date)}</p>
                </div>
                <div className="p-4 border border-border-main rounded-2xl">
                  <p className="text-sm font-bold text-text-muted mb-1 flex items-center gap-1.5"><Clock className="w-4 h-4" /> Waktu</p>
                  <p className="text-[15px] font-bold text-text-main">{booking.start_time} - {booking.end_time}</p>
                </div>
              </div>

              {/* Price */}
              <div className="p-4 border border-border-main rounded-2xl flex justify-between items-center">
                <p className="font-bold text-text-main">Total Tagihan</p>
                <p className="text-2xl font-extrabold text-primary">{formatRupiah(booking.total_price)}</p>
              </div>

              {/* Actions & Policy */}
              {booking.status === 'PENDING_PAYMENT' && (
                <div className="mt-8">
                  <div className="p-4 bg-blue-50 border border-blue-100 rounded-2xl mb-6">
                    <p className="text-sm text-blue-800 mb-2 font-bold flex items-center gap-2">
                      <AlertCircle className="w-4 h-4" /> Info Pembayaran
                    </p>
                    <p className="text-sm text-blue-700">
                      Silakan lakukan pembayaran manual dan konfirmasi dengan mengisi referensi (nama pengirim / bank).
                    </p>
                  </div>
                  
                  <div className="mb-6">
                    <label className="block text-[13px] font-extrabold text-text-main mb-2">Referensi Pembayaran</label>
                    <input 
                      type="text" 
                      placeholder="Contoh: Transfer BCA a.n Budi" 
                      value={paymentReference}
                      onChange={(e) => setPaymentReference(e.target.value)}
                      className="w-full px-4 py-3 bg-surface border border-border-main rounded-xl text-[15px] font-medium text-text-main focus:outline-none focus:border-primary focus:ring-2 focus:ring-primary/20 transition-all"
                    />
                  </div>
                  
                  <div className="flex flex-col sm:flex-row gap-3">
                    <button 
                      onClick={() => setModalState({ type: 'pay', isOpen: true })}
                      disabled={actionLoading !== null}
                      className="flex-1 bg-primary text-white py-3 rounded-xl font-bold text-[15px] hover:bg-primary/90 transition-colors disabled:opacity-50"
                    >
                      Kirim Bukti Pembayaran
                    </button>
                    <button 
                      onClick={() => setModalState({ type: 'cancel', isOpen: true })}
                      disabled={actionLoading !== null}
                      className="flex-1 bg-red-50 text-red-600 py-3 rounded-xl font-bold text-[15px] hover:bg-red-100 transition-colors disabled:opacity-50"
                    >
                      Batalkan Pesanan
                    </button>
                  </div>

                  <p className="text-xs text-text-muted mt-4 text-center">
                    * Kebijakan Pembatalan: Anda hanya dapat membatalkan pesanan jika status masih Menunggu Pembayaran. Jika sudah dikonfirmasi, pembatalan tidak diizinkan.
                  </p>
                </div>
              )}
            </div>
          </div>
        )}
      </div>

      <ConfirmModal
        isOpen={modalState.isOpen}
        title={modalState.type === 'cancel' ? 'Batalkan Pesanan?' : 'Konfirmasi Pembayaran'}
        message={modalState.type === 'cancel' ? 'Pesanan yang belum dibayar akan dibatalkan secara permanen. Apakah Anda yakin?' : 'Apakah Anda yakin ingin melakukan konfirmasi bahwa Anda telah membayar tagihan ini?'}
        confirmText={modalState.type === 'cancel' ? 'Ya, Batalkan' : 'Ya, Konfirmasi'}
        cancelText="Tutup"
        isDestructive={modalState.type === 'cancel'}
        onConfirm={modalState.type === 'cancel' ? handleCancel : handleConfirmPay}
        onCancel={() => setModalState({ type: null, isOpen: false })}
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
