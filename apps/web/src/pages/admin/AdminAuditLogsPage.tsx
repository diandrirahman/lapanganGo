import React, { useCallback, useEffect, useState } from 'react';
import { adminApi } from '../../lib/api/admin';
import type { AuditLogResponse } from '../../lib/api/admin';
import toast from 'react-hot-toast';
import { RefreshCw, Activity } from 'lucide-react';
import { format } from 'date-fns';

export const AdminAuditLogsPage: React.FC = () => {
  const [logs, setLogs] = useState<AuditLogResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const [action, setAction] = useState('');
  const [appliedAction, setAppliedAction] = useState('');
  const [entityType, setEntityType] = useState('');
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);

  const fetchLogs = useCallback(async () => {
    try {
      setLoading(true);
      const res = await adminApi.getAuditLogs({ action: appliedAction, entity_type: entityType, page, limit: 20 });
      setLogs(res.data);
      setTotalPages(res.total_pages);
    } catch (error: any) {
      toast.error(error.message || error.response?.data?.message || 'Failed to fetch audit logs');
    } finally {
      setLoading(false);
    }
  }, [appliedAction, entityType, page]);

  useEffect(() => {
    fetchLogs();
  }, [fetchLogs]);

  const handleFilter = (e: React.FormEvent) => {
    e.preventDefault();
    setAppliedAction(action);
    setPage(1);
  };

  const handleResetFilters = () => {
    setAction('');
    setAppliedAction('');
    setEntityType('');
    setPage(1);
  };

  const formatMetadata = (metadata: any) => {
    if (!metadata) return '-';
    try {
      return (
        <pre className="text-xs bg-slate-100 p-2 rounded border border-slate-200 overflow-x-auto whitespace-pre-wrap font-mono mt-1 text-slate-600">
          {JSON.stringify(metadata, null, 2)}
        </pre>
      );
    } catch {
      return String(metadata);
    }
  };

  return (
    <div className="space-y-6 max-w-7xl mx-auto">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">Audit Logs</h1>
          <p className="text-sm text-slate-500 mt-1">Platform-wide system activities</p>
        </div>
        <button
          onClick={fetchLogs}
          className="inline-flex items-center justify-center px-4 py-2 bg-white border border-slate-200 rounded-lg text-sm font-medium text-slate-700 hover:bg-slate-50 transition-colors"
        >
          <RefreshCw className="mr-2 h-4 w-4" />
          Refresh
        </button>
      </div>

      <div className="bg-white rounded-xl shadow-sm border border-slate-200 overflow-hidden">
        <div className="p-4 border-b border-slate-200 bg-slate-50 flex flex-col sm:flex-row gap-4 items-end">
          <div className="w-full sm:w-64">
            <select
              value={entityType}
              onChange={(e) => {
                setEntityType(e.target.value);
                setPage(1);
              }}
              className="block w-full px-3 py-2 text-base border border-slate-300 focus:outline-none focus:ring-2 focus:ring-emerald-500 focus:border-emerald-500 sm:text-sm rounded-lg"
            >
              <option value="">All Entities</option>
              {[
                { value: 'OWNER_PROFILE', label: 'Owner Profile' },
                { value: 'VENUE', label: 'Venue' },
                { value: 'USER', label: 'User' },
                { value: 'BOOKING', label: 'Booking' },
                { value: 'STAFF', label: 'Staff' },
                { value: 'REFUND', label: 'Refund' },
                { value: 'FINANCE_TRANSACTION', label: 'Finance Transaction' }
              ].map(opt => (
                <option key={opt.value} value={opt.value}>{opt.label}</option>
              ))}
            </select>
          </div>
          <form onSubmit={handleFilter} className="flex-1 w-full">
            <div className="flex gap-2">
              <input
                type="text"
                placeholder="Filter by action (e.g. UPDATE_STATUS)"
                value={action}
                onChange={(e) => setAction(e.target.value)}
                className="block w-full px-3 py-2 border border-slate-300 rounded-lg leading-5 bg-white placeholder-slate-400 focus:outline-none focus:ring-2 focus:ring-emerald-500 focus:border-emerald-500 sm:text-sm transition-colors"
              />
              <button
                type="submit"
                className="inline-flex items-center justify-center px-4 py-2 border border-transparent rounded-lg shadow-sm text-sm font-medium text-white bg-emerald-600 hover:bg-emerald-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-emerald-500 whitespace-nowrap"
              >
                Apply
              </button>
            </div>
          </form>
          <button
            onClick={handleResetFilters}
            className="inline-flex items-center justify-center px-4 py-2 bg-white border border-slate-300 rounded-lg text-sm font-medium text-slate-700 hover:bg-slate-50 transition-colors whitespace-nowrap"
          >
            Reset
          </button>
        </div>

        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-slate-200">
            <thead className="bg-slate-50">
              <tr>
                <th className="px-6 py-3 text-left text-xs font-medium text-slate-500 uppercase tracking-wider">
                  Event
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-slate-500 uppercase tracking-wider">
                  Actor
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-slate-500 uppercase tracking-wider">
                  Details
                </th>
              </tr>
            </thead>
            <tbody className="bg-white divide-y divide-slate-200">
              {loading ? (
                <tr>
                  <td colSpan={3} className="px-6 py-12 text-center text-slate-500">
                    <div className="flex flex-col items-center">
                      <div className="h-6 w-6 animate-spin rounded-full border-2 border-emerald-500 border-t-transparent"></div>
                      <span className="mt-2 text-sm">Loading logs...</span>
                    </div>
                  </td>
                </tr>
              ) : logs.length === 0 ? (
                <tr>
                  <td colSpan={3} className="px-6 py-12 text-center text-slate-500">
                    No logs found
                  </td>
                </tr>
              ) : (
                logs.map((log) => (
                  <tr key={log.id} className="hover:bg-slate-50">
                    <td className="px-6 py-4">
                      <div className="flex items-start">
                        <div className="mt-0.5 flex-shrink-0 h-8 w-8 bg-slate-100 rounded-lg flex items-center justify-center text-slate-500">
                          <Activity className="h-4 w-4" />
                        </div>
                        <div className="ml-3">
                          <div className="text-sm font-bold text-slate-900">{log.action}</div>
                          <div className="text-xs font-medium text-slate-500 mt-0.5 uppercase tracking-wide">
                            {log.entity_type} {log.entity_id && `(${log.entity_id.split('-')[0]}...)`}
                          </div>
                          <div className="text-xs text-slate-400 mt-1 flex items-center">
                            {format(new Date(log.created_at), 'dd MMM yyyy, HH:mm:ss')}
                          </div>
                        </div>
                      </div>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <div className="text-sm font-medium text-slate-900">
                        {log.actor_user_id || 'System'}
                      </div>
                      {log.actor_role && (
                        <div className="text-xs mt-0.5 text-slate-500">
                          Role: <span className="font-semibold">{log.actor_role}</span>
                        </div>
                      )}
                      {log.ip_address && (
                        <div className="text-xs mt-0.5 text-slate-400">
                          IP: {log.ip_address}
                        </div>
                      )}
                    </td>
                    <td className="px-6 py-4 max-w-md">
                      {formatMetadata(log.metadata)}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
        
        {/* Pagination */}
        {!loading && totalPages > 1 && (
          <div className="px-6 py-4 border-t border-slate-200 flex items-center justify-between">
            <span className="text-sm text-slate-500">
              Page {page} of {totalPages}
            </span>
            <div className="flex space-x-2">
              <button
                onClick={() => setPage(p => Math.max(1, p - 1))}
                disabled={page === 1}
                className="px-3 py-1 border border-slate-300 rounded-md text-sm font-medium text-slate-700 bg-white hover:bg-slate-50 disabled:opacity-50 disabled:cursor-not-allowed"
              >
                Previous
              </button>
              <button
                onClick={() => setPage(p => Math.min(totalPages, p + 1))}
                disabled={page === totalPages}
                className="px-3 py-1 border border-slate-300 rounded-md text-sm font-medium text-slate-700 bg-white hover:bg-slate-50 disabled:opacity-50 disabled:cursor-not-allowed"
              >
                Next
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
};
