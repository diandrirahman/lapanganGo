import React, { useCallback, useEffect, useState } from 'react';
import { adminApi } from '../../lib/api/admin';
import type { OwnerResponse } from '../../lib/api/admin';
import toast from 'react-hot-toast';
import { Search, RefreshCw, AlertCircle, CheckCircle } from 'lucide-react';
import { format } from 'date-fns';

export const AdminOwnersPage: React.FC = () => {
  const [owners, setOwners] = useState<OwnerResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [appliedSearch, setAppliedSearch] = useState('');
  const [status, setStatus] = useState('');
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [processingId, setProcessingId] = useState<string | null>(null);

  const fetchOwners = useCallback(async () => {
    try {
      setLoading(true);
      const res = await adminApi.getOwners({ search: appliedSearch, status, page, limit: 20 });
      setOwners(res.data);
      setTotalPages(res.total_pages);
    } catch (error: any) {
      toast.error(error.message || error.response?.data?.message || 'Failed to fetch owners');
    } finally {
      setLoading(false);
    }
  }, [appliedSearch, status, page]);

  useEffect(() => {
    fetchOwners();
  }, [fetchOwners]);

  const applySearch = () => {
    setAppliedSearch(search);
    setPage(1);
  };

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    applySearch();
  };

  const handleSearchKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      applySearch();
    }
  };

  const handleResetFilters = () => {
    setSearch('');
    setAppliedSearch('');
    setStatus('');
    setPage(1);
  };

  const handleUpdateStatus = async (owner: OwnerResponse, newStatus: 'ACTIVE' | 'SUSPENDED') => {
    if (!window.confirm(`Are you sure you want to ${newStatus === 'SUSPENDED' ? 'suspend' : 'activate'} this owner?`)) {
      return;
    }

    try {
      setProcessingId(owner.id);
      await adminApi.updateOwnerStatus(owner.id, newStatus);
      toast.success(`Owner ${newStatus === 'SUSPENDED' ? 'suspended' : 'activated'} successfully`);
      fetchOwners();
    } catch (error: any) {
      if (error.message === 'Request timeout' || error.name === 'AbortError') {
        toast.error('Status belum pasti (network timeout), mohon refresh halaman');
      } else {
        toast.error(error.message || error.response?.data?.message || 'Failed to update owner status');
      }
    } finally {
      setProcessingId(null);
    }
  };

  return (
    <div className="space-y-6 max-w-7xl mx-auto">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">Owners</h1>
          <p className="text-sm text-slate-500 mt-1">Manage venue owners</p>
        </div>
        <button
          onClick={fetchOwners}
          className="inline-flex items-center justify-center px-4 py-2 bg-white border border-slate-200 rounded-lg text-sm font-medium text-slate-700 hover:bg-slate-50 transition-colors"
        >
          <RefreshCw className="mr-2 h-4 w-4" />
          Refresh
        </button>
      </div>

      <div className="bg-white rounded-xl shadow-sm border border-slate-200 overflow-hidden">
        <div className="p-4 border-b border-slate-200 bg-slate-50 flex flex-col sm:flex-row gap-4 items-end">
          <form onSubmit={handleSearch} className="flex-1 w-full">
            <div className="flex gap-2">
              <input
                type="text"
                placeholder="Search by business name, owner name, or email..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                onKeyDown={handleSearchKeyDown}
                className="block w-full px-3 py-2 border border-slate-300 rounded-lg leading-5 bg-white placeholder-slate-400 focus:outline-none focus:ring-2 focus:ring-emerald-500 focus:border-emerald-500 sm:text-sm transition-colors"
              />
              <button
                type="submit"
                aria-label="Search"
                title="Search"
                className="inline-flex items-center justify-center px-4 py-2 border border-transparent rounded-lg shadow-sm text-sm font-medium text-white bg-emerald-600 hover:bg-emerald-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-emerald-500"
              >
                <Search className="h-4 w-4" />
              </button>
            </div>
          </form>
          <div className="w-full sm:w-40">
            <select
              value={status}
              onChange={(e) => {
                setStatus(e.target.value);
                setPage(1);
              }}
              className="block w-full px-3 py-2 text-base border border-slate-300 focus:outline-none focus:ring-2 focus:ring-emerald-500 focus:border-emerald-500 sm:text-sm rounded-lg"
            >
              <option value="">All Status</option>
              <option value="ACTIVE">Active</option>
              <option value="SUSPENDED">Suspended</option>
            </select>
          </div>
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
                  Business Name
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-slate-500 uppercase tracking-wider">
                  Status
                </th>
                <th className="px-6 py-3 text-left text-xs font-medium text-slate-500 uppercase tracking-wider">
                  Created At
                </th>
                <th className="px-6 py-3 text-right text-xs font-medium text-slate-500 uppercase tracking-wider">
                  Actions
                </th>
              </tr>
            </thead>
            <tbody className="bg-white divide-y divide-slate-200">
              {loading ? (
                <tr>
                  <td colSpan={4} className="px-6 py-12 text-center text-slate-500">
                    <div className="flex flex-col items-center">
                      <div className="h-6 w-6 animate-spin rounded-full border-2 border-emerald-500 border-t-transparent"></div>
                      <span className="mt-2 text-sm">Loading owners...</span>
                    </div>
                  </td>
                </tr>
              ) : owners.length === 0 ? (
                <tr>
                  <td colSpan={4} className="px-6 py-12 text-center text-slate-500">
                    No owners found
                  </td>
                </tr>
              ) : (
                owners.map((owner) => (
                  <tr key={owner.id} className="hover:bg-slate-50">
                    <td className="px-6 py-4 whitespace-nowrap">
                      <div className="text-sm font-medium text-slate-900">{owner.business_name}</div>
                      <div className="text-xs text-slate-500 font-mono mt-1">ID: {owner.id}</div>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <span className={`px-2 inline-flex text-xs leading-5 font-semibold rounded-full ${
                        owner.status === 'ACTIVE'
                          ? 'bg-emerald-100 text-emerald-800'
                          : 'bg-red-100 text-red-800'
                      }`}>
                        {owner.status}
                      </span>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-slate-500">
                      {format(new Date(owner.created_at), 'dd MMM yyyy')}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-right text-sm font-medium">
                      {owner.status === 'ACTIVE' ? (
                        <button
                          onClick={() => handleUpdateStatus(owner, 'SUSPENDED')}
                          disabled={processingId === owner.id}
                          className="inline-flex items-center text-red-600 hover:text-red-900 disabled:opacity-50"
                        >
                          <AlertCircle className="mr-1.5 h-4 w-4" />
                          Suspend
                        </button>
                      ) : (
                        <button
                          onClick={() => handleUpdateStatus(owner, 'ACTIVE')}
                          disabled={processingId === owner.id}
                          className="inline-flex items-center text-emerald-600 hover:text-emerald-900 disabled:opacity-50"
                        >
                          <CheckCircle className="mr-1.5 h-4 w-4" />
                          Activate
                        </button>
                      )}
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
