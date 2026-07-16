import React, { useCallback, useEffect, useRef, useState } from 'react';
import { AlertCircle, BadgePercent, CalendarClock, ChevronLeft, ChevronRight, RefreshCw, ShieldCheck } from 'lucide-react';
import { format } from 'date-fns';
import { adminApi } from '../../lib/api/admin';
import type {
  CommercialTermResponse,
  CommercialTermScope,
  CommercialTermStatus,
} from '../../lib/api/admin';

const DEFAULT_LIMIT = 20;
const OWNER_PROFILE_ID_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

const statusStyles: Record<CommercialTermStatus, string> = {
  CURRENT: 'bg-emerald-100 text-emerald-800',
  SCHEDULED: 'bg-sky-100 text-sky-800',
  HISTORICAL: 'bg-slate-100 text-slate-600',
};

const statusLabels: Record<CommercialTermStatus, string> = {
  CURRENT: 'Current',
  SCHEDULED: 'Scheduled',
  HISTORICAL: 'Historical · immutable history',
};

const isAbortError = (error: unknown): boolean => {
  return error instanceof DOMException && error.name === 'AbortError'
    || error instanceof Error && error.name === 'AbortError';
};

const isValidOwnerProfileID = (value: string): boolean => OWNER_PROFILE_ID_PATTERN.test(value.trim());

const formatDateTime = (value: string | null): string => {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return format(date, 'dd MMM yyyy, HH:mm');
};

const formatCommission = (bps: number): string => {
  if (!Number.isFinite(bps)) return '—';
  return `${bps} bps · ${(bps / 100).toFixed(2)}%`;
};

const shortID = (value: string | null): string => {
  if (!value) return '—';
  return value.length > 12 ? `${value.slice(0, 8)}…` : value;
};

const sourceLabel = (term: CommercialTermResponse): string => {
  return term.owner_profile_id ? `Owner-specific · ${shortID(term.owner_profile_id)}` : 'Global default';
};

const TermsLoadingState: React.FC = () => (
  <div className="space-y-3" aria-label="Loading commercial terms" role="status">
    {[1, 2, 3].map((item) => (
      <div key={item} className="h-32 animate-pulse rounded-xl border border-slate-200 bg-white" />
    ))}
  </div>
);

const TermStatus: React.FC<{ status: CommercialTermStatus }> = ({ status }) => (
  <span className={`inline-flex rounded-full px-2.5 py-1 text-xs font-semibold ${statusStyles[status]}`}>
    {statusLabels[status]}
  </span>
);

const TermDetails: React.FC<{ term: CommercialTermResponse; compact?: boolean }> = ({ term, compact = false }) => (
  <>
    <div className={compact ? 'space-y-3' : 'grid gap-4 sm:grid-cols-2 lg:grid-cols-4'}>
      <div>
        <p className="text-xs font-medium uppercase tracking-wide text-slate-400">Commission rate</p>
        <p className="mt-1 text-sm font-semibold text-slate-900">{formatCommission(term.commission_bps)}</p>
      </div>
      <div>
        <p className="text-xs font-medium uppercase tracking-wide text-slate-400">Effective window</p>
        <p className="mt-1 text-sm text-slate-700">{formatDateTime(term.valid_from)}</p>
        <p className="text-xs text-slate-500">until {term.valid_until ? formatDateTime(term.valid_until) : 'no end date'}</p>
      </div>
      <div>
        <p className="text-xs font-medium uppercase tracking-wide text-slate-400">Finance mode</p>
        <p className={`mt-1 text-sm font-semibold ${term.finance_mode === 'LIVE' ? 'text-amber-700' : 'text-slate-900'}`}>
          {term.finance_mode === 'LIVE' ? 'LIVE (read-only)' : term.finance_mode}
        </p>
        <p className="text-xs text-slate-500">Collection: {term.collection_method}</p>
      </div>
      <div>
        <p className="text-xs font-medium uppercase tracking-wide text-slate-400">Source</p>
        <p className="mt-1 text-sm text-slate-700">{sourceLabel(term)}</p>
        <p className="text-xs text-slate-500">Phase: {term.phase}</p>
      </div>
    </div>
    <div className="mt-4 flex flex-wrap gap-x-4 gap-y-1 text-xs text-slate-400">
      <span>Term ID: {shortID(term.id)}</span>
      <span>Created: {formatDateTime(term.created_at)}</span>
      {term.supersedes_id && <span>Supersedes: {shortID(term.supersedes_id)}</span>}
    </div>
  </>
);

const TermsList: React.FC<{ terms: CommercialTermResponse[] }> = ({ terms }) => (
  <>
    <div className="hidden overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm md:block">
      <div className="overflow-x-auto">
        <table className="min-w-full divide-y divide-slate-200">
          <thead className="bg-slate-50">
            <tr>
              <th className="px-5 py-3 text-left text-xs font-semibold uppercase tracking-wide text-slate-500">Term</th>
              <th className="px-5 py-3 text-left text-xs font-semibold uppercase tracking-wide text-slate-500">Status</th>
              <th className="px-5 py-3 text-left text-xs font-semibold uppercase tracking-wide text-slate-500">Rate</th>
              <th className="px-5 py-3 text-left text-xs font-semibold uppercase tracking-wide text-slate-500">Effective window</th>
              <th className="px-5 py-3 text-left text-xs font-semibold uppercase tracking-wide text-slate-500">Configuration</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {terms.map((term) => (
              <tr key={term.id} className={term.status === 'HISTORICAL' ? 'bg-slate-50/70' : 'hover:bg-slate-50'}>
                <td className="px-5 py-4 align-top">
                  <p className="font-semibold text-slate-900">{term.label}</p>
                  <p className="mt-1 text-xs text-slate-500">{sourceLabel(term)}</p>
                  <p className="mt-1 text-xs uppercase tracking-wide text-slate-400">{term.phase}</p>
                </td>
                <td className="px-5 py-4 align-top"><TermStatus status={term.status} /></td>
                <td className="px-5 py-4 align-top text-sm font-semibold text-slate-900">{formatCommission(term.commission_bps)}</td>
                <td className="px-5 py-4 align-top text-sm text-slate-700">
                  <p>{formatDateTime(term.valid_from)}</p>
                  <p className="text-xs text-slate-500">until {term.valid_until ? formatDateTime(term.valid_until) : 'no end date'}</p>
                </td>
                <td className="px-5 py-4 align-top text-sm text-slate-700">
                  <p>{term.finance_mode === 'LIVE' ? 'LIVE (read-only)' : term.finance_mode}</p>
                  <p className="text-xs text-slate-500">Collection: {term.collection_method}</p>
                  <p className="text-xs text-slate-500">Created: {formatDateTime(term.created_at)}</p>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>

    <div className="space-y-3 md:hidden">
      {terms.map((term) => (
        <article key={term.id} className={`rounded-xl border border-slate-200 bg-white p-4 shadow-sm ${term.status === 'HISTORICAL' ? 'opacity-80' : ''}`}>
          <div className="flex items-start justify-between gap-3">
            <div>
              <h2 className="font-semibold text-slate-900">{term.label}</h2>
              <p className="mt-1 text-xs text-slate-500">{sourceLabel(term)}</p>
            </div>
            <TermStatus status={term.status} />
          </div>
          <div className="mt-4"><TermDetails term={term} compact /></div>
        </article>
      ))}
    </div>
  </>
);

export const AdminCommercialTermsPage: React.FC = () => {
  const [terms, setTerms] = useState<CommercialTermResponse[]>([]);
  const [scope, setScope] = useState<CommercialTermScope>('ALL');
  const [status, setStatus] = useState<CommercialTermStatus | ''>('');
  const [ownerProfileID, setOwnerProfileID] = useState('');
  const [appliedScope, setAppliedScope] = useState<CommercialTermScope>('ALL');
  const [appliedStatus, setAppliedStatus] = useState<CommercialTermStatus | ''>('');
  const [appliedOwnerProfileID, setAppliedOwnerProfileID] = useState('');
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filterError, setFilterError] = useState<string | null>(null);
  const controllerRef = useRef<AbortController | null>(null);
  const requestIDRef = useRef(0);

  const fetchTerms = useCallback(async () => {
    controllerRef.current?.abort();
    const controller = new AbortController();
    controllerRef.current = controller;
    const requestID = ++requestIDRef.current;

    setLoading(true);
    setError(null);
    try {
      const response = await adminApi.getCommercialTerms({
        page,
        limit: DEFAULT_LIMIT,
        scope: appliedScope,
        ...(appliedScope === 'OWNER' ? { owner_profile_id: appliedOwnerProfileID } : {}),
        ...(appliedStatus ? { status: appliedStatus } : {}),
      }, { signal: controller.signal });

      if (requestID !== requestIDRef.current) return;
      setTerms(Array.isArray(response.data) ? response.data : []);
      setTotalPages(Number.isFinite(response.total_pages) ? Math.max(0, response.total_pages) : 0);
    } catch (requestError) {
      if (requestID !== requestIDRef.current || isAbortError(requestError)) return;
      setError('Commercial terms could not be loaded. Please try again.');
    } finally {
      if (requestID === requestIDRef.current) setLoading(false);
    }
  }, [appliedOwnerProfileID, appliedScope, appliedStatus, page]);

  useEffect(() => {
    void fetchTerms();
    return () => controllerRef.current?.abort();
  }, [fetchTerms]);

  const applyFilters = (event: React.FormEvent) => {
    event.preventDefault();
    if (scope === 'OWNER' && !isValidOwnerProfileID(ownerProfileID)) {
      setFilterError('Masukkan Owner Profile ID berupa UUID yang valid.');
      return;
    }
    setFilterError(null);
    setAppliedScope(scope);
    setAppliedStatus(status);
    setAppliedOwnerProfileID(scope === 'OWNER' ? ownerProfileID.trim() : '');
    setPage(1);
  };

  const resetFilters = () => {
    setScope('ALL');
    setStatus('');
    setOwnerProfileID('');
    setAppliedScope('ALL');
    setAppliedStatus('');
    setAppliedOwnerProfileID('');
    setFilterError(null);
    setPage(1);
  };

  const changePage = (nextPage: number) => {
    setPage(Math.max(1, Math.min(totalPages, nextPage)));
  };

  return (
    <div className="mx-auto max-w-7xl space-y-6">
      <header className="flex flex-col justify-between gap-4 sm:flex-row sm:items-start">
        <div>
          <div className="flex items-center gap-3">
            <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-emerald-100 text-emerald-700">
              <BadgePercent className="h-6 w-6" />
            </div>
            <div>
              <h1 className="text-2xl font-bold text-slate-900">Commercial Terms</h1>
              <p className="mt-1 text-sm text-slate-500">Read-only history of platform commission terms.</p>
            </div>
          </div>
        </div>
        <button
          type="button"
          onClick={() => void fetchTerms()}
          disabled={loading}
          className="inline-flex items-center justify-center rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-60"
        >
          <RefreshCw className={`mr-2 h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
          Refresh
        </button>
      </header>

      <section className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm" aria-label="Commercial term filters">
        <form onSubmit={applyFilters} className="grid gap-4 md:grid-cols-[1fr_1fr_1.5fr_auto_auto] md:items-end">
          <label className="block text-sm font-medium text-slate-700">
            Scope
            <select
              value={scope}
              onChange={(event) => {
                const nextScope = event.target.value as CommercialTermScope;
                setScope(nextScope);
                if (nextScope !== 'OWNER') setOwnerProfileID('');
                setFilterError(null);
              }}
              className="mt-1 block w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm focus:border-emerald-500 focus:outline-none focus:ring-2 focus:ring-emerald-500"
            >
              <option value="ALL">All terms</option>
              <option value="GLOBAL">Global defaults</option>
              <option value="OWNER">Owner-specific</option>
            </select>
          </label>

          <label className="block text-sm font-medium text-slate-700">
            Status
            <select
              value={status}
              onChange={(event) => setStatus(event.target.value as CommercialTermStatus | '')}
              className="mt-1 block w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm focus:border-emerald-500 focus:outline-none focus:ring-2 focus:ring-emerald-500"
            >
              <option value="">All statuses</option>
              <option value="CURRENT">Current</option>
              <option value="SCHEDULED">Scheduled</option>
              <option value="HISTORICAL">Historical</option>
            </select>
          </label>

          <label className="block text-sm font-medium text-slate-700">
            Owner Profile ID
            <input
              value={ownerProfileID}
              onChange={(event) => {
                setOwnerProfileID(event.target.value);
                setFilterError(null);
              }}
              disabled={scope !== 'OWNER'}
              placeholder={scope === 'OWNER' ? 'UUID required for owner scope' : 'Only used for owner scope'}
              className="mt-1 block w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm placeholder:text-slate-400 focus:border-emerald-500 focus:outline-none focus:ring-2 focus:ring-emerald-500 disabled:cursor-not-allowed disabled:bg-slate-100"
              aria-invalid={Boolean(filterError)}
            />
          </label>

          <button type="submit" className="inline-flex h-10 items-center justify-center rounded-lg bg-emerald-600 px-4 text-sm font-semibold text-white transition-colors hover:bg-emerald-700">
            Apply filters
          </button>
          <button type="button" onClick={resetFilters} className="inline-flex h-10 items-center justify-center rounded-lg border border-slate-300 bg-white px-4 text-sm font-semibold text-slate-700 transition-colors hover:bg-slate-50">
            Reset
          </button>
        </form>
        {filterError && <p className="mt-3 text-sm text-red-600" role="alert">{filterError}</p>}
      </section>

      <div className="flex items-center justify-between gap-3 text-sm text-slate-500" aria-live="polite">
        <span>{loading && terms.length > 0 ? 'Updating terms…' : `${terms.length} term${terms.length === 1 ? '' : 's'} shown`}</span>
        <span className="inline-flex items-center gap-1.5"><ShieldCheck className="h-4 w-4 text-emerald-600" /> Read-only</span>
      </div>

      {error && (
        <div className="flex flex-col gap-3 rounded-xl border border-red-200 bg-red-50 p-4 text-red-800 sm:flex-row sm:items-center sm:justify-between" role="alert">
          <div className="flex items-start gap-3">
            <AlertCircle className="mt-0.5 h-5 w-5 flex-shrink-0" />
            <p className="text-sm">{error}</p>
          </div>
          <button type="button" onClick={() => void fetchTerms()} className="rounded-lg bg-red-700 px-3 py-2 text-sm font-semibold text-white hover:bg-red-800">Retry</button>
        </div>
      )}

      {loading && terms.length === 0 ? <TermsLoadingState /> : terms.length === 0 && !error ? (
        <div className="rounded-xl border border-dashed border-slate-300 bg-white px-6 py-16 text-center">
          <CalendarClock className="mx-auto h-10 w-10 text-slate-300" />
          <h2 className="mt-4 text-lg font-semibold text-slate-800">No commercial terms found</h2>
          <p className="mt-1 text-sm text-slate-500">Try another scope or status filter.</p>
        </div>
      ) : terms.length > 0 ? <TermsList terms={terms} /> : null}

      {totalPages > 1 && (
        <nav className="flex items-center justify-between rounded-xl border border-slate-200 bg-white px-4 py-3 shadow-sm" aria-label="Commercial terms pagination">
          <span className="text-sm text-slate-500">Page {page} of {totalPages}</span>
          <div className="flex gap-2">
            <button type="button" onClick={() => changePage(page - 1)} disabled={loading || page <= 1} className="inline-flex items-center rounded-lg border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-50">
              <ChevronLeft className="mr-1 h-4 w-4" /> Previous
            </button>
            <button type="button" onClick={() => changePage(page + 1)} disabled={loading || page >= totalPages} className="inline-flex items-center rounded-lg border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-50">
              Next <ChevronRight className="ml-1 h-4 w-4" />
            </button>
          </div>
        </nav>
      )}
    </div>
  );
};
