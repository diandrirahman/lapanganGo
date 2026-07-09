import React, { useEffect, useState, useCallback } from 'react';
import { PageShell } from '../../components/layout/PageShell';
import { useAuth } from '../../contexts/AuthContext';
import { getAuditLogs } from '../../lib/api';
import type { AuditLog } from '../../types/audit';
import { AlertCircle, Calendar, Filter, FileText } from 'lucide-react';
import { useNavigate } from 'react-router-dom';

export const OwnerAuditLogsPage: React.FC = () => {
  const { token, isActualOwner } = useAuth();
  const navigate = useNavigate();
  const [logs, setLogs] = useState<AuditLog[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [limit] = useState(15);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState('');
  
  const [filterAction, setFilterAction] = useState('');
  const [filterEntity, setFilterEntity] = useState('');

  // Redirect if not actual owner
  useEffect(() => {
    if (!isActualOwner()) {
      navigate('/owner/dashboard');
    }
  }, [isActualOwner, navigate]);

  const loadLogs = useCallback(async () => {
    if (!token || !isActualOwner()) return;
    setIsLoading(true);
    setError('');
    try {
      const res = await getAuditLogs(token, {
        page,
        limit,
        action: filterAction || undefined,
        entity_type: filterEntity || undefined
      });
      setLogs(res.data || []);
      setTotal(res.total || 0);
    } catch (err: any) {
      setError(err.message || 'Gagal memuat audit logs');
    } finally {
      setIsLoading(false);
    }
  }, [token, isActualOwner, page, limit, filterAction, filterEntity]);

  useEffect(() => {
    if (isActualOwner()) {
      loadLogs();
    }
  }, [loadLogs, isActualOwner]);

  const totalPages = Math.ceil(total / limit);

  if (!isActualOwner()) {
    return null;
  }

  return (
    <PageShell>
      <div className="max-w-7xl mx-auto px-4 md:px-6 mb-8">
        <h1 className="text-3xl font-black tracking-tight text-text-main mb-2">Riwayat Aktivitas</h1>
        <p className="text-text-muted mb-6">Laporan aktivitas operasional staff dan sistem</p>
      </div>
      <div className="max-w-7xl mx-auto px-4 md:px-6 space-y-6">
        <div className="bg-white rounded-lg shadow p-4 border border-gray-100 flex flex-wrap gap-4 items-end">
          <div className="flex-1 min-w-[200px]">
            <label className="block text-sm font-medium text-gray-700 mb-1">Aksi</label>
            <div className="relative">
              <Filter className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
              <select
                value={filterAction}
                onChange={(e) => { setFilterAction(e.target.value); setPage(1); }}
                className="w-full pl-9 pr-4 py-2 bg-gray-50 border border-gray-200 rounded-lg focus:ring-2 focus:ring-green-500/20 focus:border-green-500 text-sm"
              >
                <option value="">Semua Aksi</option>
                <option value="STAFF_CREATED">STAFF_CREATED</option>
                <option value="STAFF_UPDATED">STAFF_UPDATED</option>
                <option value="STAFF_STATUS_UPDATED">STAFF_STATUS_UPDATED</option>
                <option value="STAFF_VENUES_UPDATED">STAFF_VENUES_UPDATED</option>
                <option value="BOOKING_PAYMENT_VERIFIED">BOOKING_PAYMENT_VERIFIED</option>
                <option value="BOOKING_PAYMENT_REJECTED">BOOKING_PAYMENT_REJECTED</option>
                <option value="BOOKING_MARKED_PAID">BOOKING_MARKED_PAID</option>
                <option value="BOOKING_COMPLETED">BOOKING_COMPLETED</option>
                <option value="BOOKING_CANCEL_REFUND">BOOKING_CANCEL_REFUND</option>
                <option value="REFUND_APPROVED">REFUND_APPROVED</option>
                <option value="REFUND_REJECTED">REFUND_REJECTED</option>
                <option value="FINANCE_CREATED">FINANCE_CREATED</option>
                <option value="FINANCE_UPDATED">FINANCE_UPDATED</option>
                <option value="FINANCE_DELETED">FINANCE_DELETED</option>
              </select>
            </div>
          </div>
          <div className="flex-1 min-w-[200px]">
            <label className="block text-sm font-medium text-gray-700 mb-1">Entitas</label>
            <div className="relative">
              <FileText className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
              <select
                value={filterEntity}
                onChange={(e) => { setFilterEntity(e.target.value); setPage(1); }}
                className="w-full pl-9 pr-4 py-2 bg-gray-50 border border-gray-200 rounded-lg focus:ring-2 focus:ring-green-500/20 focus:border-green-500 text-sm"
              >
                <option value="">Semua Entitas</option>
                <option value="STAFF">STAFF</option>
                <option value="BOOKING">BOOKING</option>
                <option value="REFUND">REFUND</option>
                <option value="FINANCE_TRANSACTION">FINANCE_TRANSACTION</option>
              </select>
            </div>
          </div>
        </div>

        {error && (
          <div className="p-4 bg-red-50 text-red-600 rounded-lg flex items-center gap-3">
            <AlertCircle className="w-5 h-5" />
            <p>{error}</p>
          </div>
        )}

        <div className="bg-white rounded-lg shadow overflow-hidden border border-gray-100">
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Waktu</th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Aktor</th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Aksi & Entitas</th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Metadata Tambahan</th>
                </tr>
              </thead>
              <tbody className="bg-white divide-y divide-gray-200">
                {isLoading ? (
                  <tr>
                    <td colSpan={4} className="px-6 py-12 text-center text-gray-500">
                      <div className="flex justify-center mb-2">
                        <div className="w-6 h-6 border-2 border-green-500 border-t-transparent rounded-full animate-spin"></div>
                      </div>
                      Memuat data...
                    </td>
                  </tr>
                ) : logs.length === 0 ? (
                  <tr>
                    <td colSpan={4} className="px-6 py-12 text-center text-gray-500">
                      Tidak ada riwayat aktivitas ditemukan.
                    </td>
                  </tr>
                ) : (
                  logs.map((log) => (
                    <tr key={log.id} className="hover:bg-gray-50">
                      <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                        <div className="flex items-center gap-2 text-gray-900">
                          <Calendar className="w-4 h-4 text-gray-400" />
                          {new Date(log.created_at).toLocaleString('id-ID', {
                            dateStyle: 'medium',
                            timeStyle: 'short'
                          })}
                        </div>
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap">
                        <div className="text-sm font-medium text-gray-900">{log.actor?.name || 'Sistem'}</div>
                        <div className="text-xs text-gray-500">{log.actor?.email}</div>
                        <span className="inline-flex items-center px-2 py-0.5 mt-1 rounded text-xs font-medium bg-gray-100 text-gray-800">
                          {log.actor?.role}
                        </span>
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap">
                        <div className="text-sm font-medium text-gray-900 bg-blue-50 text-blue-700 px-2 py-1 rounded inline-block">
                          {log.action}
                        </div>
                        <div className="text-xs text-gray-500 mt-1">
                          {log.entity_type} {log.entity_id ? `(${log.entity_id.substring(0,8)}...)` : ''}
                        </div>
                      </td>
                      <td className="px-6 py-4 text-sm text-gray-500 max-w-xs overflow-hidden">
                        <pre className="text-xs bg-gray-50 p-2 rounded border border-gray-100 overflow-x-auto whitespace-pre-wrap">
                          {JSON.stringify(log.metadata, null, 2)}
                        </pre>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>

          {totalPages > 1 && (
            <div className="px-6 py-4 bg-gray-50 border-t border-gray-200 flex items-center justify-between">
              <div className="text-sm text-gray-500">
                Halaman {page} dari {totalPages} ({total} total)
              </div>
              <div className="flex gap-2">
                <button
                  onClick={() => setPage(p => Math.max(1, p - 1))}
                  disabled={page === 1}
                  className="px-3 py-1 bg-white border border-gray-300 rounded text-sm disabled:opacity-50"
                >
                  Sebelumnya
                </button>
                <button
                  onClick={() => setPage(p => Math.min(totalPages, p + 1))}
                  disabled={page === totalPages}
                  className="px-3 py-1 bg-white border border-gray-300 rounded text-sm disabled:opacity-50"
                >
                  Selanjutnya
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </PageShell>
  );
};
