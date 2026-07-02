import React, { useEffect, useState, useCallback } from 'react';
import { useAuth } from '../../contexts/AuthContext';
import { fetchOwnerRefundRequests, approveRefundRequest, rejectRefundRequest } from '../../lib/api';
import type { OwnerRefundRequest, PaginatedOwnerRefundRequests } from '../../types/refund';
import { LayoutDashboard, CheckCircle, XCircle, Clock, AlertCircle } from 'lucide-react';
import { LoadingState } from '../../components/feedback/LoadingState';
import { ErrorState } from '../../components/feedback/ErrorState';
import { formatRupiah, formatDate } from '../../lib/utils';
import toast from 'react-hot-toast';
import { ConfirmModal } from '../../components/ui/ConfirmModal';
import { PageShell } from '../../components/layout/PageShell';

export const OwnerRefundsPage: React.FC = () => {
  const { token } = useAuth();
  
  const [data, setData] = useState<PaginatedOwnerRefundRequests | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [statusFilter, setStatusFilter] = useState('');
  
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [modalState, setModalState] = useState<{
    type: 'approve' | 'reject' | null,
    isOpen: boolean,
    request: OwnerRefundRequest | null,
    note: string,
    error?: string
  }>({ type: null, isOpen: false, request: null, note: '' });

  const loadRequests = useCallback(async () => {
    if (!token) return;
    try {
      setIsLoading(true);
      setError(null);
      const res = await fetchOwnerRefundRequests(token, page, 10, statusFilter);
      setData(res);
    } catch (err: any) {
      setError(err.message || 'Gagal memuat daftar permintaan refund');
    } finally {
      setIsLoading(false);
    }
  }, [token, page, statusFilter]);

  useEffect(() => {
    if (token) {
      loadRequests();
    }
  }, [token, loadRequests]);

  const handleAction = async () => {
    if (!token || !modalState.request || !modalState.type) return;
    
    // Approval can have empty note, reject is better to have one, but we allow both based on requirement
    if (modalState.type === 'reject' && !modalState.note.trim()) {
      setModalState(prev => ({ ...prev, error: 'Catatan penolakan wajib diisi' }));
      return;
    }

    try {
      setActionLoading(modalState.type);
      if (modalState.type === 'approve') {
        await approveRefundRequest(modalState.request.id, modalState.note, token);
        toast.success('Pengajuan refund disetujui');
      } else {
        await rejectRefundRequest(modalState.request.id, modalState.note, token);
        toast.success('Pengajuan refund ditolak');
      }
      setModalState({ type: null, isOpen: false, request: null, note: '' });
      await loadRequests();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Gagal memproses refund';
      setModalState(prev => ({ ...prev, error: msg }));
      toast.error(msg);
    } finally {
      setActionLoading(null);
    }
  };

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'PENDING':
        return <span className="px-3 py-1 bg-orange-100 text-orange-800 text-xs font-bold rounded-full flex items-center gap-1 w-fit"><Clock className="w-3 h-3" /> Menunggu</span>;
      case 'APPROVED':
        return <span className="px-3 py-1 bg-green-100 text-green-800 text-xs font-bold rounded-full flex items-center gap-1 w-fit"><CheckCircle className="w-3 h-3" /> Disetujui</span>;
      case 'REJECTED':
        return <span className="px-3 py-1 bg-red-100 text-red-800 text-xs font-bold rounded-full flex items-center gap-1 w-fit"><XCircle className="w-3 h-3" /> Ditolak</span>;
      case 'CANCELLED':
        return <span className="px-3 py-1 bg-gray-100 text-gray-800 text-xs font-bold rounded-full flex items-center gap-1 w-fit"><XCircle className="w-3 h-3" /> Dibatalkan</span>;
      default:
        return <span className="px-3 py-1 bg-gray-100 text-gray-800 text-xs font-bold rounded-full w-fit">{status}</span>;
    }
  };

  return (
    <PageShell>
      <div className="p-8 max-w-7xl mx-auto space-y-8">
      <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4">
        <div>
          <h1 className="text-3xl font-extrabold text-text-main flex items-center gap-3">
            <LayoutDashboard className="w-8 h-8 text-primary" />
            Permintaan Refund
          </h1>
          <p className="text-text-muted mt-2 font-medium">
            Kelola pengajuan pembatalan dan refund dari pelanggan
          </p>
        </div>
      </div>

      <div className="flex flex-wrap gap-2 items-center bg-white p-2 rounded-2xl border border-border-main shadow-sm w-fit">
        <button
          onClick={() => { setStatusFilter(''); setPage(1); }}
          className={`px-5 py-2.5 rounded-xl font-bold text-[14px] transition-all ${statusFilter === '' ? 'bg-text-main text-white' : 'text-text-muted hover:bg-surface'}`}
        >
          Semua
        </button>
        <button
          onClick={() => { setStatusFilter('PENDING'); setPage(1); }}
          className={`px-5 py-2.5 rounded-xl font-bold text-[14px] transition-all flex items-center gap-2 ${statusFilter === 'PENDING' ? 'bg-orange-100 text-orange-800' : 'text-text-muted hover:bg-surface'}`}
        >
          <Clock className="w-4 h-4" /> Menunggu
        </button>
        <button
          onClick={() => { setStatusFilter('APPROVED'); setPage(1); }}
          className={`px-5 py-2.5 rounded-xl font-bold text-[14px] transition-all flex items-center gap-2 ${statusFilter === 'APPROVED' ? 'bg-green-100 text-green-800' : 'text-text-muted hover:bg-surface'}`}
        >
          <CheckCircle className="w-4 h-4" /> Disetujui
        </button>
      </div>

      {isLoading ? (
        <LoadingState message="Memuat daftar permintaan refund..." className="bg-white rounded-3xl p-8 border border-border-main" />
      ) : error ? (
        <ErrorState message={error} onRetry={loadRequests} />
      ) : !data || data.data.length === 0 ? (
        <div className="bg-white rounded-3xl p-12 border border-border-main text-center shadow-sm">
          <div className="w-20 h-20 bg-surface rounded-2xl flex items-center justify-center mx-auto mb-6">
            <AlertCircle className="w-10 h-10 text-text-muted" />
          </div>
          <h3 className="text-xl font-bold text-text-main mb-2">Belum ada permintaan refund</h3>
          <p className="text-text-muted font-medium max-w-sm mx-auto">
            {statusFilter 
              ? 'Tidak ada permintaan refund dengan status yang dipilih.' 
              : 'Belum ada pelanggan yang mengajukan refund.'}
          </p>
        </div>
      ) : (
        <div className="bg-white rounded-3xl border border-border-main shadow-sm overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-left border-collapse">
              <thead>
                <tr className="bg-surface border-b border-border-main">
                  <th className="p-4 font-bold text-text-muted text-sm whitespace-nowrap">Tanggal & Jadwal</th>
                  <th className="p-4 font-bold text-text-muted text-sm whitespace-nowrap">Pelanggan</th>
                  <th className="p-4 font-bold text-text-muted text-sm whitespace-nowrap">Venue & Lapangan</th>
                  <th className="p-4 font-bold text-text-muted text-sm whitespace-nowrap text-right">Nilai Tagihan</th>
                  <th className="p-4 font-bold text-text-muted text-sm whitespace-nowrap text-center">Status</th>
                  <th className="p-4 font-bold text-text-muted text-sm whitespace-nowrap">Alasan</th>
                  <th className="p-4 font-bold text-text-muted text-sm whitespace-nowrap text-center">Aksi</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border-main">
                {data.data.map((req) => (
                  <tr key={req.id} className="hover:bg-surface/50 transition-colors">
                    <td className="p-4">
                      <p className="font-bold text-text-main text-sm">{formatDate(req.booking_date)}</p>
                      <p className="text-xs text-text-muted font-medium mt-1">{req.start_time} - {req.end_time}</p>
                      <p className="text-xs text-text-muted mt-1">Diajukan: {formatDate(req.requested_at)}</p>
                    </td>
                    <td className="p-4">
                      <p className="font-bold text-text-main text-sm">{req.customer_name}</p>
                      <p className="text-xs text-text-muted mt-1">{req.customer_email}</p>
                    </td>
                    <td className="p-4">
                      <p className="font-bold text-text-main text-sm truncate max-w-[200px]" title={req.venue_name}>{req.venue_name}</p>
                      <p className="text-xs text-text-muted mt-1 truncate max-w-[200px]" title={req.court_name}>{req.court_name}</p>
                    </td>
                    <td className="p-4 text-right">
                      <p className="font-extrabold text-text-main text-[15px]">{formatRupiah(req.amount)}</p>
                    </td>
                    <td className="p-4">
                      <div className="flex justify-center">
                        {getStatusBadge(req.status)}
                      </div>
                    </td>
                    <td className="p-4">
                      <p className="text-sm text-text-main max-w-xs truncate" title={req.reason}>
                        {req.reason}
                      </p>
                    </td>
                    <td className="p-4">
                      {req.status === 'PENDING' ? (
                        <div className="flex items-center justify-center gap-2">
                          <button
                            onClick={() => setModalState({ type: 'approve', isOpen: true, request: req, note: '' })}
                            className="bg-green-50 text-green-700 hover:bg-green-100 px-3 py-1.5 rounded-lg text-xs font-bold transition-colors"
                          >
                            Setujui
                          </button>
                          <button
                            onClick={() => setModalState({ type: 'reject', isOpen: true, request: req, note: '' })}
                            className="bg-red-50 text-red-700 hover:bg-red-100 px-3 py-1.5 rounded-lg text-xs font-bold transition-colors"
                          >
                            Tolak
                          </button>
                        </div>
                      ) : (
                        <div className="flex justify-center">
                          <span className="text-xs text-text-muted font-medium">-</span>
                        </div>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          
          {/* Pagination Controls */}
          {data.total_pages > 1 && (
            <div className="p-6 border-t border-border-main flex items-center justify-between bg-surface/30">
              <p className="text-sm font-medium text-text-muted">
                Halaman {data.page} dari {data.total_pages}
              </p>
              <div className="flex gap-2">
                <button
                  onClick={() => setPage(p => Math.max(1, p - 1))}
                  disabled={data.page === 1}
                  className="px-4 py-2 rounded-xl font-bold text-sm bg-white border border-border-main text-text-main hover:bg-surface disabled:opacity-50 transition-colors"
                >
                  Sebelumnya
                </button>
                <button
                  onClick={() => setPage(p => Math.min(data.total_pages, p + 1))}
                  disabled={data.page === data.total_pages}
                  className="px-4 py-2 rounded-xl font-bold text-sm bg-white border border-border-main text-text-main hover:bg-surface disabled:opacity-50 transition-colors"
                >
                  Selanjutnya
                </button>
              </div>
            </div>
          )}
        </div>
      )}

      {/* Approve/Reject Modal */}
      {modalState.isOpen && modalState.request && modalState.type && (
        <div className="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50 p-4">
          <div className="bg-white rounded-[32px] p-8 max-w-md w-full shadow-2xl relative">
            <h2 className="text-2xl font-extrabold text-text-main mb-2">
              {modalState.type === 'approve' ? 'Setujui Refund' : 'Tolak Refund'}
            </h2>
            <p className="text-text-muted mb-4">
              Booking: <span className="font-bold">{formatDate(modalState.request.booking_date)} {modalState.request.start_time}</span>
              <br/>
              Pemesan: <span className="font-bold">{modalState.request.customer_name}</span>
            </p>
            <div className="p-4 bg-orange-50 border border-orange-100 rounded-xl mb-4">
              <p className="text-xs font-bold text-orange-800 mb-1">Alasan Pelanggan:</p>
              <p className="text-sm text-orange-900">{modalState.request.reason}</p>
            </div>
            
            <div className="mb-6">
              <label className="block text-sm font-bold text-text-main mb-2">
                Catatan Anda {modalState.type === 'reject' && <span className="text-red-500">*</span>}
              </label>
              <textarea
                value={modalState.note}
                onChange={(e) => setModalState(prev => ({ ...prev, note: e.target.value }))}
                placeholder={modalState.type === 'approve' ? "Opsional: Masukkan catatan pesetujuan..." : "Wajib: Masukkan alasan penolakan..."}
                className="w-full px-4 py-3 bg-surface border border-border-main rounded-2xl text-[15px] font-medium text-text-main focus:outline-none focus:border-primary focus:ring-2 focus:ring-primary/20 transition-all min-h-[100px] resize-y"
              />
            </div>
            
            <div className="flex gap-3">
              <button
                onClick={() => setModalState({ type: null, isOpen: false, request: null, note: '' })}
                className="flex-1 px-4 py-3 rounded-2xl font-bold text-[15px] bg-surface text-text-main hover:bg-border-main transition-colors"
                disabled={actionLoading !== null}
              >
                Batal
              </button>
              <button
                onClick={handleAction}
                disabled={actionLoading !== null}
                className={`flex-1 px-4 py-3 rounded-2xl font-bold text-[15px] text-white transition-colors disabled:opacity-50 ${
                  modalState.type === 'approve' 
                    ? 'bg-green-600 hover:bg-green-700' 
                    : 'bg-red-600 hover:bg-red-700'
                }`}
              >
                {actionLoading !== null ? 'Memproses...' : (modalState.type === 'approve' ? 'Ya, Setujui' : 'Ya, Tolak')}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Error display for action */}
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
      </div>
    </PageShell>
  );
};
