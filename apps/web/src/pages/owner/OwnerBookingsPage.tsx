import React, { useEffect, useState, useCallback } from 'react';
import toast from 'react-hot-toast';
import { PageShell } from '../../components/layout/PageShell';
import { useAuth } from '../../contexts/AuthContext';
import { useSearchParams } from 'react-router-dom';
import { fetchOwnerGlobalBookings, fetchOwnerVenues, verifyPayment, markBookingPaid, completeBooking, cancelPaidBookingWithRefund } from '../../lib/api';
import type { OwnerBooking } from '../../types/booking';
import type { Venue } from '../../types/venue';
import { Search, Calendar, Clock, CheckCircle, XCircle, AlertCircle, Building2, Plus } from 'lucide-react';
import { ConfirmModal } from '../../components/ui/ConfirmModal';
import { LoadingState } from '../../components/feedback/LoadingState';
import { ErrorState } from '../../components/feedback/ErrorState';
import { formatRupiah, formatDate } from '../../lib/utils';
import { Pagination } from '../../components/ui/Pagination';
import { OwnerOfflineBookingModal } from '../../components/owner/OwnerOfflineBookingModal';

const getJakartaNowParts = () => {
  const formatter = new Intl.DateTimeFormat('en-CA', {
    timeZone: 'Asia/Jakarta',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  });

  const parts = formatter.formatToParts(new Date());
  const get = (type: string) => parts.find(part => part.type === type)?.value || '00';

  return {
    date: `${get('year')}-${get('month')}-${get('day')}`,
    time: `${get('hour')}:${get('minute')}:${get('second')}`,
  };
};

const normalizeTime = (value: string) => {
  if (!value) return '00:00:00';
  const [hour = '00', minute = '00', second = '00'] = value.split(':');
  return `${hour.padStart(2, '0')}:${minute.padStart(2, '0')}:${second.padStart(2, '0')}`;
};

const isBookingScheduleFinished = (bookingDate: string, endTime: string) => {
  const now = getJakartaNowParts();
  const normalizedEndTime = normalizeTime(endTime);

  if (bookingDate < now.date) return true;
  if (bookingDate > now.date) return false;
  return normalizedEndTime <= now.time;
};

export const OwnerBookingsPage: React.FC = () => {
  const { token } = useAuth();
  const [bookings, setBookings] = useState<OwnerBooking[]>([]);
  const [venues, setVenues] = useState<Venue[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [, setTimeTick] = useState(0);

  useEffect(() => {
    const timer = window.setInterval(() => setTimeTick(t => t + 1), 60_000);
    return () => window.clearInterval(timer);
  }, []);

  const [searchParams, setSearchParams] = useSearchParams();
  const parsedPage = Number.parseInt(searchParams.get('page') || '1', 10);
  const page = Number.isFinite(parsedPage) && parsedPage > 0 ? parsedPage : 1;
  const filterVenue = searchParams.get('venue_id') || '';
  const filterStatus = searchParams.get('status') || '';
  const tab = searchParams.get('tab') || '';
  const legacyScope = searchParams.get('scope') || '';
  const isUpcomingTab = tab === 'mendatang' || legacyScope === 'upcoming';
  const filterStartDate = searchParams.get('start_date') || '';
  const filterEndDate = searchParams.get('end_date') || '';
  const searchQuery = searchParams.get('q') || '';

  const [totalPages, setTotalPages] = useState(1);
  const [searchInput, setSearchInput] = useState(searchQuery);
  const hasActiveFilters = Boolean(filterVenue || filterStatus || isUpcomingTab || searchQuery || filterStartDate || filterEndDate);

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
  const [modalState, setModalState] = useState<{ type: 'verify' | 'reject' | 'mark_paid' | 'complete' | 'cancel_refund' | null, bookingId: string | null, isOpen: boolean, error?: string }>({ type: null, bookingId: null, isOpen: false });
  const [refundReason, setRefundReason] = useState('');
  const [isOfflineModalOpen, setIsOfflineModalOpen] = useState(false);


  useEffect(() => {
    setSearchInput(searchQuery);
  }, [searchQuery]);

  // Determine active tab based on URL filters.
  let activeTab = 'semua';
  if (isUpcomingTab) activeTab = 'mendatang';
  else if (filterStatus === 'WAITING_VERIFICATION') activeTab = 'butuh_verifikasi';
  else if (filterStatus === 'PENDING_PAYMENT') activeTab = 'menunggu_pembayaran';

  const handleTabClick = (tab: string) => {
    switch (tab) {
      case 'mendatang':
        updateParams({ tab: 'mendatang', scope: null, status: null, page: '1' });
        break;
      case 'butuh_verifikasi':
        updateParams({ tab: null, scope: null, status: 'WAITING_VERIFICATION', page: '1' });
        break;
      case 'menunggu_pembayaran':
        updateParams({ tab: null, scope: null, status: 'PENDING_PAYMENT', page: '1' });
        break;
      default:
        updateParams({ tab: null, scope: null, status: null, page: '1' });
        break;
    }
  };

  const handleSearchSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    updateParams({ q: searchInput.trim().length >= 2 ? searchInput.trim() : null, page: '1' });
  };

  const handleStatusChange = (value: string) => {
    updateParams({ status: value, tab: null, scope: null, page: '1' });
  };

  const handleDateChange = (key: 'start_date' | 'end_date', value: string) => {
    updateParams({ [key]: value, tab: null, scope: null, page: '1' });
  };

  const resetFilters = () => {
    setSearchInput('');
    updateParams({ venue_id: null, status: null, tab: null, scope: null, q: null, start_date: null, end_date: null, page: '1' });
  };

  const loadData = useCallback(async () => {
    if (!token) {
      setIsLoading(false);
      return;
    }

    setIsLoading(true);
    setError(null);
    try {
      // Load venues for filter once
      if (venues.length === 0) {
        const v = await fetchOwnerVenues(token);
        setVenues(v);
      }

      // Fetch bookings
      const data = await fetchOwnerGlobalBookings(token, {
        venue_id: filterVenue || undefined,
        status: filterStatus || undefined,
        scope: isUpcomingTab ? 'upcoming' : undefined,
        start_date: filterStartDate || undefined,
        end_date: filterEndDate || undefined,
        q: searchQuery || undefined,
        page,
        limit: 10
      });
      setBookings(data.data || []);
      setTotalPages(data.total_pages || 1);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Gagal memuat pesanan');
    } finally {
      setIsLoading(false);
    }
  }, [token, filterVenue, filterStatus, isUpcomingTab, filterStartDate, filterEndDate, searchQuery, page, venues.length]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  const handleAction = async () => {
    if (!token || !modalState.bookingId || !modalState.type) return;
    try {
      setActionLoading(modalState.type);
      if (modalState.type === 'verify') {
        await verifyPayment(modalState.bookingId, true, token);
        toast.success('Pembayaran berhasil diverifikasi!');
      } else if (modalState.type === 'reject') {
        await verifyPayment(modalState.bookingId, false, token);
        toast.success('Pembayaran ditolak.');
      } else if (modalState.type === 'mark_paid') {
        await markBookingPaid(modalState.bookingId, token);
        toast.success('Booking ditandai sebagai lunas!');
      } else if (modalState.type === 'complete') {
        await completeBooking(modalState.bookingId, token);
        toast.success('Booking ditandai selesai!');
      } else if (modalState.type === 'cancel_refund') {
        const trimmedReason = refundReason.trim();
        if (trimmedReason.length < 3) {
          const msg = 'Alasan refund minimal 3 karakter.';
          setModalState(prev => ({ ...prev, error: msg }));
          toast.error(msg);
          setActionLoading(null);
          return;
        }
        if (trimmedReason.length > 500) {
          const msg = 'Alasan refund maksimal 500 karakter.';
          setModalState(prev => ({ ...prev, error: msg }));
          toast.error(msg);
          setActionLoading(null);
          return;
        }
        await cancelPaidBookingWithRefund(modalState.bookingId, trimmedReason, token);
        toast.success('Booking dibatalkan dan refund dicatat!');
      }
      loadData();
      setModalState({ type: null, bookingId: null, isOpen: false });
      setRefundReason('');
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Gagal memproses aksi';
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

  const tabs = [
    { id: 'semua', label: 'Semua Pesanan' },
    { id: 'butuh_verifikasi', label: 'Butuh Verifikasi' },
    { id: 'mendatang', label: 'Mendatang' },
    { id: 'menunggu_pembayaran', label: 'Menunggu Pembayaran' }
  ];

  return (
    <PageShell>
      <div className="pt-24 pb-40 max-w-5xl mx-auto px-6">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between mb-8 gap-4">
          <div>
            <h1 className="text-3xl font-extrabold text-text-main mb-2">Semua Pesanan</h1>
            <p className="text-text-muted">Kelola dan pantau seluruh pesanan dari semua venue Anda secara terpusat.</p>
          </div>
          <button
            onClick={() => setIsOfflineModalOpen(true)}
            className="inline-flex items-center justify-center gap-2 px-5 py-2.5 bg-emerald-600 text-white font-bold rounded-xl hover:bg-emerald-700 transition-colors shadow-sm whitespace-nowrap"
          >
            <Plus className="w-5 h-5" />
            Tambah Booking Offline
          </button>
        </div>

        {/* Quick Tabs */}
        <div className="flex overflow-x-auto gap-2 pb-2 mb-6 scrollbar-hide">
          {tabs.map(tab => (
            <button
              key={tab.id}
              onClick={() => handleTabClick(tab.id)}
              className={`whitespace-nowrap px-4 py-2.5 rounded-full text-sm font-bold transition-all ${activeTab === tab.id
                  ? 'bg-primary text-white shadow-md'
                  : 'bg-white text-text-muted hover:bg-gray-100 border border-border-main'
                }`}
            >
              {tab.label}
            </button>
          ))}
        </div>

        {/* Filter Toolbar */}
        <div className="bg-white p-4 rounded-2xl border border-border-main mb-8 shadow-sm flex flex-col gap-4">
          <form onSubmit={handleSearchSubmit} className="flex gap-2">
            <div className="relative flex-1">
              <Search className="absolute left-4 top-1/2 -translate-y-1/2 text-gray-400 w-5 h-5" />
              <input
                type="text"
                placeholder="Cari ID booking, nama, email, venue, atau lapangan..."
                className="w-full pl-11 pr-4 py-2.5 rounded-xl border border-border-main focus:ring-2 focus:ring-primary focus:border-primary transition-all outline-none"
                value={searchInput}
                onChange={(e) => setSearchInput(e.target.value)}
              />
            </div>
            <button type="submit" className="px-6 py-2.5 bg-secondary text-white font-bold rounded-xl hover:bg-secondary/90 transition-colors">
              Cari
            </button>
            {hasActiveFilters && (
              <button
                type="button"
                onClick={resetFilters}
                className="px-4 py-2.5 bg-gray-100 text-text-main font-bold rounded-xl hover:bg-gray-200 transition-colors"
              >
                Reset
              </button>
            )}
          </form>

          <div className="grid grid-cols-1 sm:grid-cols-4 gap-4">
            <div className="sm:col-span-1">
              <label className="block text-xs font-bold text-text-muted mb-1 uppercase tracking-wider">Venue</label>
              <select
                className="w-full px-4 py-2 rounded-xl border border-border-main text-sm focus:ring-2 focus:ring-primary outline-none bg-white"
                value={filterVenue}
                onChange={(e) => updateParams({ venue_id: e.target.value, page: '1' })}
              >
                <option value="">Semua Venue</option>
                {venues.map(v => (
                  <option key={v.id} value={v.id}>{v.name}</option>
                ))}
              </select>
            </div>
            <div className="sm:col-span-1">
              <label className="block text-xs font-bold text-text-muted mb-1 uppercase tracking-wider">Status</label>
              <select
                className="w-full px-4 py-2 rounded-xl border border-border-main text-sm focus:ring-2 focus:ring-primary outline-none bg-white"
                value={filterStatus}
                onChange={(e) => handleStatusChange(e.target.value)}
              >
                <option value="">Semua Status</option>
                <option value="PENDING_PAYMENT">Menunggu Pembayaran</option>
                <option value="WAITING_VERIFICATION">Menunggu Verifikasi</option>
                <option value="PAID">Lunas</option>
                <option value="COMPLETED">Selesai</option>
                <option value="CANCELLED">Dibatalkan</option>
              </select>
            </div>
            <div className="sm:col-span-1">
              <label className="block text-xs font-bold text-text-muted mb-1 uppercase tracking-wider">Dari Tanggal</label>
              <input
                type="date"
                className="w-full px-4 py-2 rounded-xl border border-border-main text-sm focus:ring-2 focus:ring-primary outline-none"
                value={filterStartDate}
                onChange={(e) => handleDateChange('start_date', e.target.value)}
              />
            </div>
            <div className="sm:col-span-1">
              <label className="block text-xs font-bold text-text-muted mb-1 uppercase tracking-wider">Sampai Tanggal</label>
              <input
                type="date"
                className="w-full px-4 py-2 rounded-xl border border-border-main text-sm focus:ring-2 focus:ring-primary outline-none"
                value={filterEndDate}
                onChange={(e) => handleDateChange('end_date', e.target.value)}
              />
            </div>
          </div>
        </div>

        {error && (
          <div className="mb-8">
            <ErrorState message={error} onRetry={loadData} />
          </div>
        )}

        {isLoading && bookings.length === 0 ? (
          <div className="py-20"><LoadingState message="Memuat pesanan..." /></div>
        ) : bookings.length === 0 ? (
          <div className="bg-white rounded-3xl p-12 text-center border border-border-main shadow-sm flex flex-col items-center">
            <div className="w-20 h-20 bg-gray-50 rounded-full flex items-center justify-center mb-6">
              <Calendar className="w-10 h-10 text-gray-400" />
            </div>
            <h3 className="text-xl font-extrabold text-text-main mb-2">Tidak ada pesanan</h3>
            <p className="text-text-muted max-w-md mx-auto">
              Belum ada pesanan yang sesuai dengan filter.
              {hasActiveFilters && (
                <button onClick={resetFilters} className="text-primary font-bold ml-2 hover:underline">
                  Reset Filter
                </button>
              )}
            </p>
          </div>
        ) : (
          <div className="relative">
            {isLoading && (
              <div className="absolute inset-0 bg-white/50 backdrop-blur-[2px] z-10 flex items-start justify-center pt-20 rounded-2xl transition-all">
                <div className="bg-white text-primary px-4 py-2 rounded-full font-bold text-sm shadow-md flex items-center gap-2 animate-in fade-in zoom-in-95">
                  <div className="w-4 h-4 border-2 border-primary border-t-transparent rounded-full animate-spin"></div>
                  Memperbarui data...
                </div>
              </div>
            )}
            <div className="space-y-6">
              {bookings.map((booking) => (
                <div key={booking.id} className="bg-white rounded-2xl border border-border-main overflow-hidden shadow-sm hover:shadow-md transition-shadow relative">
                  <div className="p-6">
                    <div className="flex flex-col sm:flex-row sm:items-start justify-between gap-4 mb-6">
                      <div>
                        <div className="flex items-center gap-3 mb-2">
                          <span className="text-sm font-mono text-text-muted bg-gray-100 px-2 py-0.5 rounded-md">#{booking.id.split('-')[0]}</span>
                          {getStatusBadge(booking.status)}
                        </div>
                        <h3 className="text-lg font-extrabold text-text-main mb-1">
                          {booking.customer.name}
                        </h3>
                        <p className="text-text-muted text-sm flex flex-col sm:flex-row sm:items-center gap-1 sm:gap-3">
                          <span>{booking.customer.email}</span>
                          <span className="hidden sm:inline">&bull;</span>
                          <span>{booking.customer.phone}</span>
                        </p>
                      </div>
                      <div className="text-left sm:text-right">
                        <p className="text-sm font-bold text-text-muted mb-1">Total Pembayaran</p>
                        <p className="text-2xl font-extrabold text-primary">{formatRupiah(booking.total_price)}</p>
                      </div>
                    </div>

                    <div className="grid grid-cols-1 md:grid-cols-2 gap-6 p-4 bg-gray-50 rounded-xl mb-6">
                      <div>
                        <div className="flex items-center gap-2 text-text-muted text-sm font-bold mb-3">
                          <Building2 className="w-4 h-4 text-primary" />
                          Detail Tempat
                        </div>
                        <p className="font-bold text-text-main mb-1">{booking.venue.name}</p>
                        <p className="text-sm text-text-muted mb-2">{booking.court.name}</p>
                      </div>

                      <div>
                        <div className="flex items-center gap-2 text-text-muted text-sm font-bold mb-3">
                          <Clock className="w-4 h-4 text-secondary" />
                          Jadwal Main
                        </div>
                        <p className="font-bold text-text-main mb-1">{formatDate(booking.booking_date)}</p>
                        <p className="text-sm text-text-muted">{booking.start_time.slice(0, 5)} - {booking.end_time.slice(0, 5)}</p>
                      </div>
                    </div>

                    {booking.status === 'WAITING_VERIFICATION' && booking.payment_reference && (
                      <div className="bg-blue-50 border border-blue-100 rounded-xl p-5">
                        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
                          <div>
                            <p className="text-blue-900 font-bold mb-1">Bukti Pembayaran Diterima</p>
                            <p className="text-blue-700 text-sm">Referensi: <span className="font-mono bg-blue-100 px-1.5 py-0.5 rounded">{booking.payment_reference}</span></p>
                          </div>
                          <div className="flex gap-2 w-full sm:w-auto">
                            <button
                              onClick={() => setModalState({ type: 'reject', bookingId: booking.id, isOpen: true })}
                              disabled={actionLoading === 'verify'}
                              className="flex-1 sm:flex-none px-4 py-2 bg-white text-red-600 border border-red-200 hover:bg-red-50 font-bold rounded-xl text-sm transition-colors disabled:opacity-50"
                            >
                              Tolak
                            </button>
                            <button
                              onClick={() => setModalState({ type: 'verify', bookingId: booking.id, isOpen: true })}
                              disabled={actionLoading === 'verify'}
                              className="flex-1 sm:flex-none px-6 py-2 bg-secondary text-white hover:bg-secondary/90 font-bold rounded-xl text-sm transition-colors disabled:opacity-50"
                            >
                              Verifikasi
                            </button>
                          </div>
                        </div>
                      </div>
                    )}

                    {booking.status === 'CONFIRMED' && (
                      <div className="bg-gray-50 border-t border-border-main p-4 flex justify-end gap-2">
                        <button
                          onClick={() => setModalState({ type: 'mark_paid', bookingId: booking.id, isOpen: true })}
                          className="px-4 py-2 bg-white text-secondary border border-secondary hover:bg-blue-50 font-bold rounded-xl text-sm transition-colors"
                        >
                          Tandai Lunas
                        </button>
                      </div>
                    )}

                    {booking.status === 'PAID' && (() => {
                      const canComplete = isBookingScheduleFinished(booking.booking_date, booking.end_time);
                      return (
                        <div className="bg-gray-50 border-t border-border-main p-4 flex flex-col md:flex-row justify-end items-end md:items-center gap-3">
                          {!canComplete && (
                            <span className="text-xs font-bold text-text-muted text-right">
                              Bisa ditandai selesai setelah {booking.end_time}
                            </span>
                          )}
                          <button
                            onClick={() => {
                              setRefundReason('');
                              setModalState({ type: 'cancel_refund', bookingId: booking.id, isOpen: true });
                            }}
                            disabled={actionLoading !== null}
                            className="px-4 py-2 bg-white text-red-600 border border-red-200 hover:bg-red-50 font-bold rounded-xl text-sm transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                          >
                            Batalkan & Refund
                          </button>
                          <button
                            onClick={() => {
                              if (!canComplete) return;
                              setModalState({ type: 'complete', bookingId: booking.id, isOpen: true });
                            }}
                            disabled={!canComplete || actionLoading !== null}
                            className="px-4 py-2 bg-secondary text-white hover:bg-secondary/90 font-bold rounded-xl text-sm transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                          >
                            Tandai Selesai
                          </button>
                        </div>
                      );
                    })()}
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}

        {totalPages > 1 && (
          <div className="mt-8">
            <Pagination page={page} totalPages={totalPages} onPageChange={setPage} />
          </div>
        )}
      </div>

      <ConfirmModal
        isOpen={modalState.isOpen}
        title={
          modalState.type === 'verify' ? 'Verifikasi Pembayaran' :
            modalState.type === 'reject' ? 'Tolak Pembayaran' :
              modalState.type === 'mark_paid' ? 'Tandai Lunas' :
                modalState.type === 'cancel_refund' ? 'Batalkan & Refund Booking' :
                  modalState.type === 'complete' ? 'Tandai Selesai' : ''
        }
        message={
          modalState.type === 'verify' ? 'Apakah Anda yakin dana sudah masuk dan ingin memverifikasi pembayaran ini? Status akan berubah menjadi Lunas.' :
            modalState.type === 'reject' ? 'Apakah Anda yakin ingin menolak pembayaran ini? Booking akan dibatalkan.' :
              modalState.type === 'mark_paid' ? 'Tandai booking ini sebagai Lunas? Ledger pendapatan akan dibuat jika belum ada.' :
                modalState.type === 'cancel_refund' ? (
                  <div className="text-left space-y-4">
                    <p className="text-text-main">
                      Tindakan ini akan membatalkan booking, mengubah statusnya, dan mencatat Expense (pengeluaran) untuk refund di Ledger Anda.
                    </p>
                    <div>
                      <label className="block text-sm font-bold text-text-main mb-1">
                        Alasan Refund
                      </label>
                      <textarea
                        value={refundReason}
                        onChange={(e) => setRefundReason(e.target.value)}
                        placeholder="Contoh: Lapangan sedang perbaikan"
                        className="w-full px-3 py-2 border border-border-main rounded-xl focus:ring-2 focus:ring-primary focus:border-primary outline-none transition-all text-sm resize-none"
                        rows={3}
                        maxLength={500}
                      />
                      <p className="mt-1 text-xs text-text-muted text-right">
                        {refundReason.length}/500
                      </p>
                    </div>
                  </div>
                ) :
                  modalState.type === 'complete' ? 'Pastikan jadwal sudah selesai dimainkan.' : ''
        }
        confirmText={
          modalState.type === 'verify' ? 'Ya, Verifikasi' :
            modalState.type === 'reject' ? 'Ya, Tolak' :
              modalState.type === 'mark_paid' ? 'Ya, Tandai Lunas' :
                modalState.type === 'cancel_refund' ? 'Batalkan & Refund' :
                  modalState.type === 'complete' ? 'Ya, Selesai' : ''
        }
        isDestructive={modalState.type === 'reject' || modalState.type === 'cancel_refund'}
        onConfirm={handleAction}
        onCancel={() => setModalState({ type: null, bookingId: null, isOpen: false })}
        isLoading={actionLoading !== null}
      />

      <OwnerOfflineBookingModal
        isOpen={isOfflineModalOpen}
        onClose={() => setIsOfflineModalOpen(false)}
        onSuccess={() => {
          toast.success('Booking offline berhasil ditambahkan!');
          loadData();
        }}
        venues={venues}
        token={token || ''}
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
