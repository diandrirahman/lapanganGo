import React, { useEffect, useState, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import toast from 'react-hot-toast';
import { PageShell } from '../components/layout/PageShell';
import { useAuth } from '../contexts/AuthContext';
import { fetchBookingById, cancelBooking, submitPaymentProof, fetchRefundRequestByBooking, createRefundRequest } from '../lib/api';
import type { Booking } from '../types/booking';
import type { RefundRequest } from '../types/refund';
import { Calendar, Clock, MapPin, CheckCircle, AlertCircle, XCircle, ChevronLeft } from 'lucide-react';
import { LoadingState } from '../components/feedback/LoadingState';
import { ErrorState } from '../components/feedback/ErrorState';
import { ConfirmModal } from '../components/ui/ConfirmModal';
import { formatRupiah, formatDate } from '../lib/utils';
import { formatPaymentDeadline, getRemainingPaymentMs, formatRemainingPaymentTime } from '../lib/paymentExpiry';

export const CustomerBookingDetailPage: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { token } = useAuth();
  
  const [booking, setBooking] = useState<Booking | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [paymentReference, setPaymentReference] = useState('');
  const [modalState, setModalState] = useState<{ type: 'cancel' | 'pay' | 'refund' | null, isOpen: boolean, error?: string }>({ type: null, isOpen: false });
  const [refundRequest, setRefundRequest] = useState<RefundRequest | null>(null);
  const [refundReason, setRefundReason] = useState('');
  
  const [_now, setNow] = useState<Date>(new Date());

  const loadBooking = useCallback(async () => {
    if (!token || !id) return;
    try {
      setIsLoading(true);
      setError(null);
      const data = await fetchBookingById(id, token);
      setBooking(data);

      if (data.status === 'PAID' || data.status === 'CANCELLED') {
        try {
          const refund = await fetchRefundRequestByBooking(id, token);
          setRefundRequest(refund);
        } catch (e) {
          console.error("Failed to fetch refund request", e);
        }
      }
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

  useEffect(() => {
    if (booking?.status === 'PENDING_PAYMENT' && booking?.expires_at) {
      setNow(new Date());
      const interval = setInterval(() => {
        setNow(new Date());
      }, 1000);
      return () => clearInterval(interval);
    }
  }, [booking?.status, booking?.expires_at]);

  const remainingMs = booking?.expires_at ? getRemainingPaymentMs(booking.expires_at) : null;
  const isPaymentExpired = booking?.status === 'PENDING_PAYMENT' && remainingMs !== null && remainingMs <= 0;

  const prevExpiredRef = React.useRef(isPaymentExpired);
  useEffect(() => {
    if (isPaymentExpired && !prevExpiredRef.current) {
      loadBooking();
    }
    prevExpiredRef.current = isPaymentExpired;
  }, [isPaymentExpired, loadBooking]);

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
    if (isPaymentExpired) {
      toast.error('Batas pembayaran telah lewat');
      await loadBooking();
      setModalState({ type: null, isOpen: false });
      return;
    }
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

  const handleRequestRefund = async () => {
    if (!token || !id) return;
    if (refundReason.trim().length < 10) {
      setModalState(prev => ({ ...prev, error: 'Alasan refund minimal 10 karakter' }));
      return;
    }
    try {
      setActionLoading('refund');
      await createRefundRequest(id, refundReason, token);
      toast.success('Pengajuan refund berhasil dikirim');
      await loadBooking();
      setModalState({ type: null, isOpen: false });
      setRefundReason('');
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Gagal mengajukan refund';
      setModalState(prev => ({ ...prev, error: msg }));
      toast.error(msg);
    } finally {
      setActionLoading(null);
    }
  };

  const canRequestRefund = () => {
    if (!booking) return false;
    const now = new Date();
    const [hours, minutes] = booking.start_time.split(':').map(Number);
    const startDateTime = new Date(booking.booking_date);
    startDateTime.setHours(hours, minutes, 0, 0);
    const oneHourBefore = new Date(startDateTime.getTime() - 60 * 60 * 1000);
    return now < oneHourBefore;
  };

	const getStatusBadge = (status: string) => {
    switch (status) {
      case 'PENDING_PAYMENT':
        return <span className="px-4 py-1.5 bg-yellow-100 text-yellow-800 text-sm font-bold rounded-full flex items-center gap-1.5 w-fit"><AlertCircle className="w-4 h-4" /> Menunggu Pembayaran</span>;
      case 'WAITING_VERIFICATION':
        return <span className="px-4 py-1.5 bg-blue-100 text-blue-800 text-sm font-bold rounded-full flex items-center gap-1.5 w-fit"><Clock className="w-4 h-4" /> Menunggu Verifikasi</span>;
      case 'PAID':
        return <span className="px-4 py-1.5 bg-green-100 text-green-800 text-sm font-bold rounded-full flex items-center gap-1.5 w-fit"><CheckCircle className="w-4 h-4" /> Lunas</span>;
      case 'COMPLETED':
        return <span className="px-4 py-1.5 bg-gray-100 text-gray-800 text-sm font-bold rounded-full flex items-center gap-1.5 w-fit"><CheckCircle className="w-4 h-4" /> Selesai</span>;
      case 'CANCELLED':
        return <span className="px-4 py-1.5 bg-red-100 text-red-800 text-sm font-bold rounded-full flex items-center gap-1.5 w-fit"><XCircle className="w-4 h-4" /> Dibatalkan</span>;
      case 'CONFIRMED':
        return <span className="px-4 py-1.5 bg-blue-100 text-blue-800 text-sm font-bold rounded-full flex items-center gap-1.5 w-fit"><CheckCircle className="w-4 h-4" /> Dikonfirmasi</span>;
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
                  {booking.expires_at ? (
                    isPaymentExpired ? (
                      <div className="p-4 bg-red-50 border border-red-100 rounded-2xl mb-6">
                        <p className="text-sm text-red-800 mb-2 font-bold flex items-center gap-2">
                          <AlertCircle className="w-4 h-4" /> Batas Pembayaran Lewat
                        </p>
                        <p className="text-sm text-red-700">
                          Pesanan ini sudah melewati batas pembayaran. Slot dapat dipilih customer lain.
                        </p>
                      </div>
                    ) : remainingMs !== null && remainingMs <= 5 * 60 * 1000 ? (
                      <div className="p-4 bg-orange-50 border border-orange-100 rounded-2xl mb-6">
                        <p className="text-sm text-orange-800 mb-2 font-bold flex items-center gap-2">
                          <AlertCircle className="w-4 h-4" /> Waktu Pembayaran Hampir Habis
                        </p>
                        <p className="text-sm text-orange-800 mb-4 font-medium">
                          Selesaikan pembayaran sebelum {formatPaymentDeadline(booking.expires_at)}
                        </p>
                        <div className="mb-4">
                          <p className="text-xs text-orange-700 mb-1">Sisa waktu pembayaran</p>
                          <p className="text-2xl font-extrabold text-orange-800 font-mono tracking-wider">
                            {formatRemainingPaymentTime(remainingMs)}
                          </p>
                        </div>
                        <p className="text-sm text-orange-700">
                          Segera kirim referensi pembayaran agar slot Anda tidak otomatis dibatalkan.
                        </p>
                      </div>
                    ) : (
                      <div className="p-4 bg-blue-50 border border-blue-100 rounded-2xl mb-6">
                        <p className="text-sm text-blue-800 mb-2 font-bold flex items-center gap-2">
                          <AlertCircle className="w-4 h-4" /> Menunggu Pembayaran
                        </p>
                        <p className="text-sm text-blue-800 mb-4 font-medium">
                          Selesaikan pembayaran sebelum {formatPaymentDeadline(booking.expires_at)}
                        </p>
                        <div className="mb-4">
                          <p className="text-xs text-blue-700 mb-1">Sisa waktu pembayaran</p>
                          <p className="text-2xl font-extrabold text-blue-800 font-mono tracking-wider">
                            {formatRemainingPaymentTime(remainingMs!)}
                          </p>
                        </div>
                        <p className="text-sm text-blue-700">
                          Silakan lakukan pembayaran manual dan kirim referensi pembayaran agar owner dapat melakukan verifikasi.
                        </p>
                      </div>
                    )
                  ) : (
                    <div className="p-4 bg-blue-50 border border-blue-100 rounded-2xl mb-6">
                      <p className="text-sm text-blue-800 mb-2 font-bold flex items-center gap-2">
                        <AlertCircle className="w-4 h-4" /> Info Pembayaran
                      </p>
                      <p className="text-sm text-blue-700">
                        Selesaikan pembayaran dalam batas waktu yang ditentukan.<br/>
                        Silakan lakukan pembayaran manual dan kirim referensi pembayaran agar owner dapat melakukan verifikasi.
                      </p>
                    </div>
                  )}
                  
                  <div className="mb-6">
                    <label className="block text-[13px] font-extrabold text-text-main mb-2">Referensi Pembayaran</label>
                    <input 
                      type="text" 
                      placeholder="Contoh: Transfer BCA a.n Budi" 
                      value={paymentReference}
                      onChange={(e) => setPaymentReference(e.target.value)}
                      disabled={isPaymentExpired}
                      className="w-full px-4 py-3 bg-surface border border-border-main rounded-xl text-[15px] font-medium text-text-main focus:outline-none focus:border-primary focus:ring-2 focus:ring-primary/20 transition-all disabled:opacity-50"
                    />
                  </div>
                  
                  <div className="flex flex-col sm:flex-row gap-3">
                    <button 
                      onClick={() => setModalState({ type: 'pay', isOpen: true })}
                      disabled={actionLoading !== null || isPaymentExpired}
                      className="flex-1 bg-primary text-white py-3 rounded-xl font-bold text-[15px] hover:bg-primary/90 transition-colors disabled:opacity-50"
                    >
                      Kirim Bukti Pembayaran
                    </button>
                    <button 
                      onClick={() => isPaymentExpired ? navigate('/bookings') : setModalState({ type: 'cancel', isOpen: true })}
                      disabled={actionLoading !== null}
                      className="flex-1 bg-red-50 text-red-600 py-3 rounded-xl font-bold text-[15px] hover:bg-red-100 transition-colors disabled:opacity-50"
                    >
                      {isPaymentExpired ? 'Kembali' : 'Batalkan Pesanan'}
                    </button>
                  </div>

                  <p className="text-xs text-text-muted mt-4 text-center">
                    * Kebijakan Pembatalan: Anda hanya dapat membatalkan pesanan saat status masih Menunggu Pembayaran. Setelah bukti pembayaran dikirim atau pembayaran diverifikasi, pembatalan diproses oleh owner sesuai kebijakan venue.
                  </p>
                </div>
              )}

              {booking.status === 'WAITING_VERIFICATION' && (
                <div className="mt-8 p-4 bg-blue-50 border border-blue-100 rounded-2xl">
                  <p className="text-sm text-blue-800 mb-2 font-bold flex items-center gap-2">
                    <Clock className="w-4 h-4" /> Verifikasi Pembayaran
                  </p>
                  <p className="text-sm text-blue-700">
                    Bukti pembayaran sudah dikirim. Pesanan Anda sedang menunggu verifikasi dari owner.
                  </p>
                  {booking.payment_reference && (
                    <p className="text-sm text-blue-700 mt-2 font-medium">
                      Referensi: {booking.payment_reference}
                    </p>
                  )}
                </div>
              )}

              {booking.status === 'PAID' && (
                <div className="mt-8">
                  <div className="p-4 bg-green-50 border border-green-100 rounded-2xl mb-6">
                    <p className="text-sm text-green-800 mb-2 font-bold flex items-center gap-2">
                      <CheckCircle className="w-4 h-4" /> Pembayaran Berhasil
                    </p>
                    <p className="text-sm text-green-700">
                      Pembayaran sudah diverifikasi. Booking Anda sudah lunas dan siap digunakan sesuai jadwal.
                    </p>
                  </div>
                  
                  {refundRequest ? (
                    <div className="p-4 bg-orange-50 border border-orange-200 rounded-2xl">
                      <p className="text-sm text-orange-800 mb-2 font-bold flex items-center gap-2">
                        <AlertCircle className="w-4 h-4" /> Pengajuan Refund
                      </p>
                      <p className="text-sm text-orange-700 mb-2">
                        Anda telah mengajukan refund untuk pesanan ini.
                      </p>
                      <div className="bg-white p-3 rounded-xl border border-orange-100 text-sm mb-3">
                        <span className="font-bold text-orange-800">Status: </span>
                        <span className="font-semibold">{refundRequest.status}</span>
                      </div>
                      <div className="bg-white p-3 rounded-xl border border-orange-100 text-sm">
                        <span className="font-bold text-orange-800">Alasan: </span>
                        <span>{refundRequest.reason}</span>
                      </div>
                      {refundRequest.owner_note && (
                        <div className="bg-white p-3 rounded-xl border border-orange-100 text-sm mt-3">
                          <span className="font-bold text-orange-800">Catatan Owner: </span>
                          <span>{refundRequest.owner_note}</span>
                        </div>
                      )}
                    </div>
                  ) : canRequestRefund() ? (
                    <div className="p-4 border border-border-main rounded-2xl bg-surface">
                      <p className="text-sm font-bold text-text-main mb-2">Perubahan Jadwal?</p>
                      <p className="text-xs text-text-muted mb-4">
                        Anda dapat mengajukan pembatalan & refund hingga maksimal 1 jam sebelum jadwal mulai. Persetujuan dan jumlah dana yang dikembalikan bergantung pada kebijakan masing-masing venue.
                      </p>
                      <button 
                        onClick={() => setModalState({ type: 'refund', isOpen: true })}
                        className="w-full bg-white border-2 border-border-main text-text-main py-2.5 rounded-xl font-bold text-[14px] hover:border-text-main transition-colors"
                      >
                        Ajukan Batalkan & Refund
                      </button>
                    </div>
                  ) : (
                    <div className="p-4 bg-gray-50 rounded-2xl">
                      <p className="text-xs text-text-muted text-center italic">
                        Batas waktu pengajuan refund telah lewat (maksimal 1 jam sebelum jadwal mulai).
                      </p>
                    </div>
                  )}
                </div>
              )}

              {booking.status === 'COMPLETED' && (
                <div className="mt-8 p-4 bg-gray-50 border border-gray-200 rounded-2xl">
                  <p className="text-sm text-gray-800 mb-2 font-bold flex items-center gap-2">
                    <CheckCircle className="w-4 h-4" /> Pesanan Selesai
                  </p>
                  <p className="text-sm text-gray-700">
                    Booking ini sudah selesai.
                  </p>
                </div>
              )}

              {booking.status === 'CANCELLED' && (
                <div className="mt-8">
                  <div className="p-4 bg-red-50 border border-red-100 rounded-2xl mb-6">
                    <p className="text-sm text-red-800 mb-2 font-bold flex items-center gap-2">
                      <XCircle className="w-4 h-4" /> Pesanan Dibatalkan
                    </p>
                    <p className="text-sm text-red-700">
                      Booking ini telah dibatalkan.
                    </p>
                    {!refundRequest && (
                      <p className="text-sm text-red-700 mt-2">
                        Jika booking sudah lunas, refund diproses oleh owner sesuai kebijakan venue.
                      </p>
                    )}
                  </div>

                  {refundRequest && (
                    <div className="p-4 bg-orange-50 border border-orange-200 rounded-2xl">
                      <p className="text-sm text-orange-800 mb-2 font-bold flex items-center gap-2">
                        <AlertCircle className="w-4 h-4" /> Detail Refund
                      </p>
                      <div className="bg-white p-3 rounded-xl border border-orange-100 text-sm mb-3">
                        <span className="font-bold text-orange-800">Status: </span>
                        <span className="font-semibold">{refundRequest.status}</span>
                      </div>
                      <div className="bg-white p-3 rounded-xl border border-orange-100 text-sm">
                        <span className="font-bold text-orange-800">Alasan: </span>
                        <span>{refundRequest.reason}</span>
                      </div>
                      {refundRequest.owner_note && (
                        <div className="bg-white p-3 rounded-xl border border-orange-100 text-sm mt-3">
                          <span className="font-bold text-orange-800">Catatan Owner: </span>
                          <span>{refundRequest.owner_note}</span>
                        </div>
                      )}
                    </div>
                  )}
                </div>
              )}

              {booking.status === 'CONFIRMED' && (
                <div className="mt-8 p-4 bg-blue-50 border border-blue-100 rounded-2xl">
                  <p className="text-sm text-blue-800 mb-2 font-bold flex items-center gap-2">
                    <CheckCircle className="w-4 h-4" /> Status Legacy
                  </p>
                  <p className="text-sm text-blue-700">
                    Booking ini berstatus dikonfirmasi pada data lama. Jika diperlukan, owner akan menandai booking sebagai lunas.
                  </p>
                </div>
              )}
            </div>
          </div>
        )}
      </div>

      <ConfirmModal
        isOpen={modalState.isOpen && (modalState.type === 'cancel' || modalState.type === 'pay')}
        title={modalState.type === 'cancel' ? 'Batalkan Pesanan?' : 'Konfirmasi Pembayaran'}
        message={modalState.type === 'cancel' ? 'Pesanan yang belum dibayar akan dibatalkan secara permanen. Apakah Anda yakin?' : 'Apakah Anda yakin ingin melakukan konfirmasi bahwa Anda telah membayar tagihan ini?'}
        confirmText={modalState.type === 'cancel' ? 'Ya, Batalkan' : 'Ya, Konfirmasi'}
        cancelText="Tutup"
        isDestructive={modalState.type === 'cancel'}
        onConfirm={modalState.type === 'cancel' ? handleCancel : handleConfirmPay}
        onCancel={() => setModalState({ type: null, isOpen: false })}
        isLoading={actionLoading !== null}
      />

      {modalState.isOpen && modalState.type === 'refund' && (
        <div className="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50 p-4">
          <div className="bg-white rounded-[32px] p-8 max-w-md w-full shadow-2xl relative">
            <h2 className="text-2xl font-extrabold text-text-main mb-2">Ajukan Refund</h2>
            <p className="text-text-muted mb-6">
              Jelaskan alasan Anda membatalkan pesanan. Dana akan dikembalikan sesuai dengan kebijakan venue jika disetujui.
            </p>
            <div className="mb-6">
              <label className="block text-sm font-bold text-text-main mb-2">Alasan Pembatalan</label>
              <textarea
                value={refundReason}
                onChange={(e) => setRefundReason(e.target.value)}
                placeholder="Contoh: Jadwal mendadak bentrok dengan acara keluarga..."
                className="w-full px-4 py-3 bg-surface border border-border-main rounded-2xl text-[15px] font-medium text-text-main focus:outline-none focus:border-primary focus:ring-2 focus:ring-primary/20 transition-all min-h-[120px] resize-y"
              />
            </div>
            <div className="flex gap-3">
              <button
                onClick={() => setModalState({ type: null, isOpen: false })}
                className="flex-1 px-4 py-3 rounded-2xl font-bold text-[15px] bg-surface text-text-main hover:bg-border-main transition-colors"
                disabled={actionLoading !== null}
              >
                Kembali
              </button>
              <button
                onClick={handleRequestRefund}
                disabled={actionLoading !== null}
                className="flex-1 px-4 py-3 rounded-2xl font-bold text-[15px] bg-red-600 text-white hover:bg-red-700 transition-colors disabled:opacity-50"
              >
                {actionLoading === 'refund' ? 'Memproses...' : 'Kirim Pengajuan'}
              </button>
            </div>
          </div>
        </div>
      )}
      
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
