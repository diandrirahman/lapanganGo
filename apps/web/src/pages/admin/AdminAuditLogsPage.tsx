import React, { useCallback, useEffect, useRef, useState } from 'react';
import { Activity, AlertCircle, ChevronLeft, ChevronRight, RefreshCw, ShieldCheck } from 'lucide-react';
import { format } from 'date-fns';
import { adminApi } from '../../lib/api/admin';
import type { AuditLogResponse, AuditScope } from '../../lib/api/admin';

const PAGE_LIMIT = 20;

const entityOptions = [
  { value: 'OWNER_PROFILE', label: 'Owner profile', scope: 'OWNER' },
  { value: 'VENUE', label: 'Venue', scope: 'OWNER' },
  { value: 'USER', label: 'User', scope: 'OWNER' },
  { value: 'BOOKING', label: 'Booking', scope: 'OWNER' },
  { value: 'STAFF', label: 'Staff', scope: 'OWNER' },
  { value: 'REFUND', label: 'Refund', scope: 'OWNER' },
  { value: 'FINANCE_TRANSACTION', label: 'Finance transaction', scope: 'OWNER' },
  { value: 'PLATFORM_COMMERCIAL_TERM', label: 'Platform commercial term', scope: 'PLATFORM' },
  { value: 'PLATFORM_FINANCE_JOURNAL', label: 'Platform finance journal', scope: 'PLATFORM' },
  { value: 'PLATFORM_EXPENSE', label: 'Platform expense', scope: 'PLATFORM' },
] as const;

const actionOptions = [
  { value: 'STAFF_CREATED', scope: 'OWNER' },
  { value: 'STAFF_UPDATED', scope: 'OWNER' },
  { value: 'STAFF_STATUS_UPDATED', scope: 'OWNER' },
  { value: 'STAFF_VENUES_UPDATED', scope: 'OWNER' },
  { value: 'STAFF_INVITE_CREATED', scope: 'OWNER' },
  { value: 'STAFF_INVITE_REGENERATED', scope: 'OWNER' },
  { value: 'STAFF_PASSWORD_RESET_REQUESTED', scope: 'OWNER' },
  { value: 'STAFF_PASSWORD_RESET_COMPLETED', scope: 'OWNER' },
  { value: 'STAFF_PASSWORD_SETUP_COMPLETED', scope: 'OWNER' },
  { value: 'BOOKING_PAYMENT_VERIFIED', scope: 'OWNER' },
  { value: 'BOOKING_PAYMENT_REJECTED', scope: 'OWNER' },
  { value: 'BOOKING_MARKED_PAID', scope: 'OWNER' },
  { value: 'BOOKING_COMPLETED', scope: 'OWNER' },
  { value: 'BOOKING_CANCEL_REFUND', scope: 'OWNER' },
  { value: 'REFUND_APPROVED', scope: 'OWNER' },
  { value: 'REFUND_REJECTED', scope: 'OWNER' },
  { value: 'FINANCE_CREATED', scope: 'OWNER' },
  { value: 'FINANCE_UPDATED', scope: 'OWNER' },
  { value: 'FINANCE_DELETED', scope: 'OWNER' },
  { value: 'UPDATE_OWNER_STATUS', scope: 'OWNER' },
  { value: 'UPDATE_VENUE_STATUS', scope: 'OWNER' },
  { value: 'PLATFORM_COMMERCIAL_TERM_CREATED', scope: 'PLATFORM' },
  { value: 'PLATFORM_COMMERCIAL_TERM_SUPERSEDED', scope: 'PLATFORM' },
  { value: 'PLATFORM_COMMERCIAL_TERM_LIVE_REJECTED', scope: 'PLATFORM' },
  { value: 'PLATFORM_FINANCE_JOURNAL_REVERSED', scope: 'PLATFORM' },
  { value: 'PLATFORM_FINANCE_LIVE_WRITE_REJECTED', scope: 'PLATFORM' },
  { value: 'PLATFORM_EXPENSE_CREATED', scope: 'PLATFORM' },
] as const;

const isAbortError = (error: unknown): boolean => (
  error instanceof DOMException && error.name === 'AbortError'
) || (error instanceof Error && error.name === 'AbortError');

const isScopeCompatible = (optionScope: 'OWNER' | 'PLATFORM', selectedScope: AuditScope): boolean => (
  selectedScope === 'ALL' || optionScope === selectedScope
);

const formatDateTime = (value: string): string => {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '—' : format(date, 'dd MMM yyyy, HH:mm:ss');
};

const shortID = (value?: string): string => {
  if (!value) return '—';
  return value.length > 12 ? `${value.slice(0, 8)}…` : value;
};

const ownerLabel = (log: AuditLogResponse): string => (
  log.owner_profile_id ? `Owner ${shortID(log.owner_profile_id)}` : 'No owner association'
);

const metadataEntries = (metadata: unknown): Array<[string, string]> => {
  if (!metadata || typeof metadata !== 'object' || Array.isArray(metadata)) return [];
  return Object.entries(metadata)
    .filter(([key, value]) => (
      /^[a-z][a-z0-9_]{0,63}$/i.test(key)
      && (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean')
    ))
    .slice(0, 8)
    .map(([key, value]) => [key.replaceAll('_', ' '), String(value).slice(0, 240)]);
};

const MetadataSummary: React.FC<{ metadata: unknown }> = ({ metadata }) => {
  const entries = metadataEntries(metadata);
  if (entries.length === 0) return <span className="text-sm text-slate-400">No safe metadata available</span>;

  return (
    <dl className="grid gap-x-4 gap-y-1 text-xs sm:grid-cols-2">
      {entries.map(([key, value]) => (
        <div key={key} className="min-w-0">
          <dt className="inline font-medium capitalize text-slate-500">{key}: </dt>
          <dd className="inline break-words text-slate-700">{value}</dd>
        </div>
      ))}
    </dl>
  );
};

const AuditLogList: React.FC<{ logs: AuditLogResponse[] }> = ({ logs }) => (
  <>
    <div className="hidden overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm md:block">
      <div className="overflow-x-auto">
        <table className="min-w-full divide-y divide-slate-200">
          <thead className="bg-slate-50">
            <tr>
              {['Event', 'Scope & owner', 'Actor', 'Safe metadata'].map((heading) => (
                <th key={heading} className="px-5 py-3 text-left text-xs font-semibold uppercase tracking-wide text-slate-500">{heading}</th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {logs.map((log) => (
              <tr key={`${log.scope}-${log.id}`} className="hover:bg-slate-50">
                <td className="px-5 py-4 align-top">
                  <p className="break-words text-sm font-semibold text-slate-900">{log.action}</p>
                  <p className="mt-1 text-xs font-medium uppercase tracking-wide text-slate-500">{log.entity_type} · {shortID(log.entity_id)}</p>
                  <p className="mt-2 text-xs text-slate-400">{formatDateTime(log.created_at)}</p>
                </td>
                <td className="px-5 py-4 align-top text-sm text-slate-700">
                  <p><span className="rounded-full bg-slate-100 px-2 py-1 text-xs font-semibold text-slate-600">{log.scope}</span></p>
                  <p className="mt-2 text-xs text-slate-500">{ownerLabel(log)}</p>
                  {log.venue_id && <p className="mt-1 text-xs text-slate-400">Venue {shortID(log.venue_id)}</p>}
                </td>
                <td className="px-5 py-4 align-top text-sm text-slate-700">
                  <p className="font-medium text-slate-900">{log.actor_user_id ? shortID(log.actor_user_id) : 'System'}</p>
                  <p className="mt-1 text-xs text-slate-500">{log.actor_role || 'No role recorded'}</p>
                </td>
                <td className="max-w-md px-5 py-4 align-top"><MetadataSummary metadata={log.metadata} /></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>

    <div className="space-y-3 md:hidden">
      {logs.map((log) => (
        <article key={`${log.scope}-${log.id}`} className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <h2 className="break-words text-sm font-semibold text-slate-900">{log.action}</h2>
              <p className="mt-1 text-xs uppercase tracking-wide text-slate-500">{log.entity_type} · {shortID(log.entity_id)}</p>
            </div>
            <span className="shrink-0 rounded-full bg-slate-100 px-2 py-1 text-xs font-semibold text-slate-600">{log.scope}</span>
          </div>
          <div className="mt-4 space-y-1 text-xs text-slate-500">
            <p>{formatDateTime(log.created_at)}</p>
            <p>{ownerLabel(log)}</p>
            <p>Actor: {log.actor_user_id ? shortID(log.actor_user_id) : 'System'} · {log.actor_role || 'No role recorded'}</p>
          </div>
          <div className="mt-4 border-t border-slate-100 pt-3"><MetadataSummary metadata={log.metadata} /></div>
        </article>
      ))}
    </div>
  </>
);

export const AdminAuditLogsPage: React.FC = () => {
  const [logs, setLogs] = useState<AuditLogResponse[]>([]);
  const [scope, setScope] = useState<AuditScope>('ALL');
  const [entityType, setEntityType] = useState('');
  const [action, setAction] = useState('');
  const [appliedScope, setAppliedScope] = useState<AuditScope>('ALL');
  const [appliedEntityType, setAppliedEntityType] = useState('');
  const [appliedAction, setAppliedAction] = useState('');
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const controllerRef = useRef<AbortController | null>(null);
  const requestIDRef = useRef(0);

  const fetchLogs = useCallback(async () => {
    controllerRef.current?.abort();
    const controller = new AbortController();
    controllerRef.current = controller;
    const requestID = ++requestIDRef.current;

    setLoading(true);
    setError(null);
    try {
      const response = await adminApi.getAuditLogs({
        scope: appliedScope,
        entity_type: appliedEntityType,
        action: appliedAction,
        page,
        limit: PAGE_LIMIT,
      }, { signal: controller.signal });
      if (requestID !== requestIDRef.current) return;
      setLogs(Array.isArray(response.data) ? response.data : []);
      setTotalPages(Number.isFinite(response.total_pages) ? Math.max(0, response.total_pages) : 0);
    } catch (requestError) {
      if (requestID !== requestIDRef.current || isAbortError(requestError)) return;
      setError('Audit logs could not be loaded. Please try again.');
    } finally {
      if (requestID === requestIDRef.current) setLoading(false);
    }
  }, [appliedAction, appliedEntityType, appliedScope, page]);

  useEffect(() => {
    void fetchLogs();
    return () => controllerRef.current?.abort();
  }, [fetchLogs]);

  const applyFilters = (event: React.FormEvent) => {
    event.preventDefault();
    setAppliedScope(scope);
    setAppliedEntityType(entityType);
    setAppliedAction(action);
    setPage(1);
  };

  const resetFilters = () => {
    setScope('ALL');
    setEntityType('');
    setAction('');
    setAppliedScope('ALL');
    setAppliedEntityType('');
    setAppliedAction('');
    setPage(1);
  };

  const updateScope = (nextScope: AuditScope) => {
    setScope(nextScope);
    if (entityType && !isScopeCompatible(entityOptions.find((option) => option.value === entityType)?.scope ?? 'OWNER', nextScope)) setEntityType('');
    if (action && !isScopeCompatible(actionOptions.find((option) => option.value === action)?.scope ?? 'OWNER', nextScope)) setAction('');
  };

  const changePage = (nextPage: number) => setPage(Math.max(1, Math.min(totalPages, nextPage)));
  const visibleEntities = entityOptions.filter((option) => isScopeCompatible(option.scope, scope));
  const visibleActions = actionOptions.filter((option) => isScopeCompatible(option.scope, scope));

  return (
    <div className="mx-auto max-w-7xl space-y-6">
      <header className="flex flex-col justify-between gap-4 sm:flex-row sm:items-start">
        <div className="flex items-center gap-3">
          <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-emerald-100 text-emerald-700"><Activity className="h-6 w-6" /></div>
          <div>
            <h1 className="text-2xl font-bold text-slate-900">Platform Audit</h1>
            <p className="mt-1 text-sm text-slate-500">Read-only platform and owner audit events.</p>
          </div>
        </div>
        <button type="button" onClick={() => void fetchLogs()} disabled={loading} className="inline-flex items-center justify-center rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-60">
          <RefreshCw className={`mr-2 h-4 w-4 ${loading ? 'animate-spin' : ''}`} /> Refresh
        </button>
      </header>

      <section className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm" aria-label="Platform audit filters">
        <form onSubmit={applyFilters} className="grid gap-4 md:grid-cols-4 md:items-end">
          <label className="block text-sm font-medium text-slate-700">Scope
            <select value={scope} onChange={(event) => updateScope(event.target.value as AuditScope)} className="mt-1 block w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm focus:border-emerald-500 focus:outline-none focus:ring-2 focus:ring-emerald-500">
              <option value="ALL">All audit events</option><option value="OWNER">Owner events</option><option value="PLATFORM">Platform events</option>
            </select>
          </label>
          <label className="block text-sm font-medium text-slate-700">Entity
            <select value={entityType} onChange={(event) => setEntityType(event.target.value)} className="mt-1 block w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm focus:border-emerald-500 focus:outline-none focus:ring-2 focus:ring-emerald-500">
              <option value="">All entities</option>{visibleEntities.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
            </select>
          </label>
          <label className="block text-sm font-medium text-slate-700">Action
            <select value={action} onChange={(event) => setAction(event.target.value)} className="mt-1 block w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm focus:border-emerald-500 focus:outline-none focus:ring-2 focus:ring-emerald-500">
              <option value="">All actions</option>{visibleActions.map((option) => <option key={option.value} value={option.value}>{option.value}</option>)}
            </select>
          </label>
          <div className="flex gap-2">
            <button type="submit" className="inline-flex h-10 flex-1 items-center justify-center rounded-lg bg-emerald-600 px-4 text-sm font-semibold text-white hover:bg-emerald-700">Apply filters</button>
            <button type="button" onClick={resetFilters} className="inline-flex h-10 items-center justify-center rounded-lg border border-slate-300 bg-white px-4 text-sm font-semibold text-slate-700 hover:bg-slate-50">Reset</button>
          </div>
        </form>
      </section>

      <div className="flex items-center justify-between gap-3 text-sm text-slate-500" aria-live="polite">
        <span>{loading && logs.length > 0 ? 'Updating audit logs…' : `${logs.length} event${logs.length === 1 ? '' : 's'} shown`}</span>
        <span className="inline-flex items-center gap-1.5"><ShieldCheck className="h-4 w-4 text-emerald-600" /> Read-only, scalar metadata only</span>
      </div>

      {error && <div className="flex flex-col gap-3 rounded-xl border border-red-200 bg-red-50 p-4 text-red-800 sm:flex-row sm:items-center sm:justify-between" role="alert"><div className="flex items-start gap-3"><AlertCircle className="mt-0.5 h-5 w-5 shrink-0" /><p className="text-sm">{error}</p></div><button type="button" onClick={() => void fetchLogs()} className="rounded-lg bg-red-700 px-3 py-2 text-sm font-semibold text-white hover:bg-red-800">Retry</button></div>}

      {loading && logs.length === 0 ? <div className="space-y-3" role="status" aria-label="Loading audit logs">{[1, 2, 3].map((item) => <div key={item} className="h-28 animate-pulse rounded-xl border border-slate-200 bg-white" />)}</div> : logs.length === 0 && !error ? <div className="rounded-xl border border-dashed border-slate-300 bg-white px-6 py-16 text-center"><Activity className="mx-auto h-10 w-10 text-slate-300" /><h2 className="mt-4 text-lg font-semibold text-slate-800">No audit events found</h2><p className="mt-1 text-sm text-slate-500">Try another scope, entity, or action filter.</p></div> : logs.length > 0 ? <AuditLogList logs={logs} /> : null}

      {totalPages > 1 && <nav className="flex items-center justify-between rounded-xl border border-slate-200 bg-white px-4 py-3 shadow-sm" aria-label="Audit log pagination"><span className="text-sm text-slate-500">Page {page} of {totalPages}</span><div className="flex gap-2"><button type="button" onClick={() => changePage(page - 1)} disabled={loading || page <= 1} className="inline-flex items-center rounded-lg border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-50"><ChevronLeft className="mr-1 h-4 w-4" />Previous</button><button type="button" onClick={() => changePage(page + 1)} disabled={loading || page >= totalPages} className="inline-flex items-center rounded-lg border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-50">Next<ChevronRight className="ml-1 h-4 w-4" /></button></div></nav>}
    </div>
  );
};
