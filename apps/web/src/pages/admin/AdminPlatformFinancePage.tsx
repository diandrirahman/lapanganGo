import React, { useEffect, useMemo, useRef, useState } from 'react';
import { AlertCircle, CircleDollarSign, ExternalLink, Filter, RefreshCw, TrendingUp } from 'lucide-react';
import { Link, useSearchParams } from 'react-router-dom';
import { adminApi, type OwnerResponse, type VenueResponse } from '../../lib/api/admin';
import { chartIntegerPercent, formatCalendarDate, formatIntegerRupiah, formatTrendPeriod, parseIntegerRupiah } from '../../lib/platformFinance';
import type { PlatformFinanceSummaryQuery, PlatformFinanceSummaryResponse } from '../../types/platformFinance';

const FILTER_KEYS = ['start_date', 'end_date', 'owner_profile_id', 'venue_id', 'granularity'] as const;

function readFilters(params: URLSearchParams): PlatformFinanceSummaryQuery {
  const result: PlatformFinanceSummaryQuery = {};
  FILTER_KEYS.forEach((key) => {
    const value = params.get(key);
    if (value) result[key] = value as never;
  });
  return result;
}

function formatActual(value: string | null): string {
  return value === null ? 'Belum tersedia' : formatIntegerRupiah(value);
}

const MetricCard: React.FC<{ label: string; value: string; tone?: 'actual' | 'projection' | 'neutral'; hint?: string; testID?: string }> = ({ label, value, tone = 'neutral', hint, testID }) => (
  <article className={`rounded-xl border p-4 shadow-sm ${tone === 'projection' ? 'border-indigo-200 bg-indigo-50' : tone === 'actual' ? 'border-emerald-200 bg-emerald-50' : 'border-slate-200 bg-white'}`}>
    <p className="text-xs font-semibold uppercase tracking-wide text-slate-500">{label}</p>
    <p data-testid={testID} className="mt-2 text-xl font-bold text-slate-900">{value}</p>
    {hint && <p className="mt-2 text-xs text-slate-600">{hint}</p>}
  </article>
);

const TrendPanel: React.FC<{ summary: PlatformFinanceSummaryResponse }> = ({ summary }) => {
  const trend = summary.trend;
  const maxAbs = trend.reduce((max, item) => {
    const value = parseIntegerRupiah(item.platform_operating_expense);
    if (value === null) return max;
    const absolute = value < 0n ? -value : value;
    return absolute > max ? absolute : max;
  }, 0n);
  return (
    <section className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm" data-testid="platform-finance-trend">
      <div className="flex flex-col justify-between gap-2 sm:flex-row sm:items-start">
        <div>
          <h2 className="flex items-center gap-2 text-lg font-semibold text-slate-900"><TrendingUp className="h-5 w-5 text-emerald-600" />OPEX Trend</h2>
          <p className="mt-1 text-sm text-slate-500">Effective period: {summary.granularity}. Nilai tabel adalah sumber audit exact.</p>
        </div>
        <span className="rounded-full bg-emerald-100 px-2.5 py-1 text-xs font-semibold text-emerald-800">AVAILABLE</span>
      </div>
      {trend.length === 0 ? <p className="mt-5 rounded-lg border border-dashed border-slate-300 p-6 text-center text-sm text-slate-500">Tidak ada bucket pada periode ini.</p> : <>
        <div className="mt-5 space-y-3" aria-label="OPEX visual trend">
          {trend.map((item) => {
            const amount = parseIntegerRupiah(item.platform_operating_expense);
            const width = chartIntegerPercent(item.platform_operating_expense, maxAbs);
            return <div key={`${item.period_start}-${item.period_end}`} className="grid grid-cols-[7rem_1fr_auto] items-center gap-3 text-xs sm:grid-cols-[10rem_1fr_auto]">
              <span className="truncate text-slate-600">{formatTrendPeriod(item.period_start, item.period_end)}</span>
              <div className="h-2 rounded-full bg-slate-100"><div className={`h-2 rounded-full ${amount !== null && amount < 0n ? 'bg-amber-500' : 'bg-emerald-500'}`} style={{ width: `${width}%` }} /></div>
              <span className="whitespace-nowrap font-semibold text-slate-800">{formatIntegerRupiah(item.platform_operating_expense)}</span>
            </div>;
          })}
        </div>
        <div className="mt-6 overflow-x-auto rounded-lg border border-slate-200">
          <table className="min-w-full divide-y divide-slate-200 text-sm">
            <thead className="bg-slate-50"><tr><th className="px-3 py-2 text-left font-semibold text-slate-600">Periode</th><th className="px-3 py-2 text-right font-semibold text-slate-600">OPEX Platform</th><th className="px-3 py-2 text-right font-semibold text-slate-600">Proyeksi Komisi</th></tr></thead>
            <tbody className="divide-y divide-slate-100">{trend.map((item) => <tr key={`row-${item.period_start}-${item.period_end}`}><td className="px-3 py-2 text-slate-700">{formatTrendPeriod(item.period_start, item.period_end)}</td><td className="px-3 py-2 text-right font-semibold text-slate-900">{formatIntegerRupiah(item.platform_operating_expense)}</td><td className="px-3 py-2 text-right text-indigo-800">{formatIntegerRupiah(item.projected_commission)}</td></tr>)}</tbody>
          </table>
        </div>
      </>}
    </section>
  );
};

export const AdminPlatformFinancePage: React.FC = () => {
  const [searchParams, setSearchParams] = useSearchParams();
  const filterKey = searchParams.toString();
  const filters = useMemo(() => readFilters(searchParams), [searchParams]);
  const [summary, setSummary] = useState<PlatformFinanceSummaryResponse | null>(null);
  const [ownerSearch, setOwnerSearch] = useState('');
  const [ownerPage, setOwnerPage] = useState(1);
  const [owners, setOwners] = useState<OwnerResponse[]>([]);
  const [ownerTotalPages, setOwnerTotalPages] = useState(0);
  const [venues, setVenues] = useState<VenueResponse[]>([]);
  const [ownerOptionsError, setOwnerOptionsError] = useState<string | null>(null);
  const [venueOptionsError, setVenueOptionsError] = useState<string | null>(null);
  const [optionsRetryToken, setOptionsRetryToken] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshToken, setRefreshToken] = useState(0);
  const controllerRef = useRef<AbortController | null>(null);
  const requestIDRef = useRef(0);

  useEffect(() => {
    let active = true;
    setOwnerOptionsError(null);
    const timer = window.setTimeout(() => {
      void adminApi.getOwners({ page: ownerPage, limit: 25, ...(ownerSearch.trim() ? { search: ownerSearch.trim() } : {}) }).then((response) => {
        if (!active) return;
        setOwners(response.data);
        setOwnerTotalPages(response.total_pages ?? 0);
      }).catch(() => {
        if (active) setOwnerOptionsError('Daftar owner tidak dapat dimuat.');
      });
    }, 250);
    return () => { active = false; window.clearTimeout(timer); };
  }, [ownerPage, ownerSearch, optionsRetryToken]);

  useEffect(() => {
    let active = true;
    setVenueOptionsError(null);
    void adminApi.getVenues({ page: 1, limit: 100, ...(filters.owner_profile_id ? { owner_profile_id: filters.owner_profile_id } : {}) }).then((response) => { if (active) setVenues(response.data); }).catch(() => {
      if (active) setVenueOptionsError('Daftar venue tidak dapat dimuat.');
    });
    return () => { active = false; };
  }, [filters.owner_profile_id, optionsRetryToken]);

  useEffect(() => {
    controllerRef.current?.abort();
    const controller = new AbortController();
    controllerRef.current = controller;
    const requestID = ++requestIDRef.current;
    setLoading(true);
    setError(null);
    void adminApi.getPlatformFinanceSummary(filters, { signal: controller.signal }).then((response) => {
      if (requestID === requestIDRef.current) setSummary(response);
    }).catch((requestError: unknown) => {
      if (controller.signal.aborted || requestID !== requestIDRef.current) return;
      setSummary(null);
      setError(requestError instanceof Error ? requestError.message : 'Finance summary could not be loaded.');
    }).finally(() => {
      if (requestID === requestIDRef.current) setLoading(false);
    });
    return () => controller.abort();
  }, [filters, filterKey, refreshToken]);

  const updateFilters = (patch: Partial<PlatformFinanceSummaryQuery>) => {
    const next = { ...filters, ...patch };
    Object.keys(next).forEach((key) => {
      if (!next[key as keyof PlatformFinanceSummaryQuery]) delete next[key as keyof PlatformFinanceSummaryQuery];
    });
    const params = new URLSearchParams();
    Object.entries(next).forEach(([key, value]) => { if (value) params.set(key, String(value)); });
    setSearchParams(params);
  };

  const refresh = () => setRefreshToken((current) => current + 1);
  const retryOptions = () => {
    setOwnerOptionsError(null);
    setVenueOptionsError(null);
    setOptionsRetryToken((current) => current + 1);
  };

  const ownerSelectionMissing = filters.owner_profile_id && !owners.some((owner) => owner.id === filters.owner_profile_id);
  const scopedProjectionUnavailable = Boolean(filters.owner_profile_id || filters.venue_id) && summary?.metrics.projected_operating_result_before_transaction_costs === null;
  const optionsError = ownerOptionsError ?? venueOptionsError;

  return <div className="mx-auto max-w-7xl space-y-6">
    <header className="flex flex-col justify-between gap-4 sm:flex-row sm:items-start"><div className="flex items-center gap-3"><div className="flex h-11 w-11 items-center justify-center rounded-xl bg-emerald-100 text-emerald-700"><CircleDollarSign className="h-6 w-6" /></div><div><h1 className="text-2xl font-bold text-slate-900">Keuangan Platform</h1><p className="mt-1 text-sm text-slate-500">Ringkasan OPEX dan tren berdasarkan journal immutable.</p></div></div><div className="flex flex-wrap gap-2"><Link to="/admin/finance/expenses" className="inline-flex items-center rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50">Pengeluaran <ExternalLink className="ml-2 h-4 w-4" /></Link><button type="button" onClick={refresh} disabled={loading} className="inline-flex items-center rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-60"><RefreshCw className={`mr-2 h-4 w-4 ${loading ? 'animate-spin' : ''}`} />Refresh</button></div></header>
    <div className="rounded-xl border border-indigo-200 bg-indigo-50 p-4 text-sm text-indigo-950"><p className="font-semibold">MODE SIMULASI</p><p className="mt-1">Komisi dan operating result pada halaman ini adalah proyeksi. LapangGo belum memotong atau menerima komisi dari owner.</p></div>
    <section className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm"><div className="mb-3 flex items-center gap-2 text-sm font-semibold text-slate-700"><Filter className="h-4 w-4" />Filter laporan</div><div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5"><label className="text-sm font-medium text-slate-700">Mulai<input type="date" value={filters.start_date ?? ''} onChange={(event) => updateFilters({ start_date: event.target.value })} className="mt-1 block w-full rounded-lg border border-slate-200 px-3 py-2" /></label><label className="text-sm font-medium text-slate-700">Sampai<input type="date" value={filters.end_date ?? ''} onChange={(event) => updateFilters({ end_date: event.target.value })} className="mt-1 block w-full rounded-lg border border-slate-200 px-3 py-2" /></label><div><label className="text-sm font-medium text-slate-700" htmlFor="owner-search">Cari owner</label><input id="owner-search" aria-label="Owner search" value={ownerSearch} onChange={(event) => { setOwnerSearch(event.target.value); setOwnerPage(1); }} placeholder="Nama owner" className="mt-1 block w-full rounded-lg border border-slate-200 px-3 py-2 text-sm" /><label className="mt-2 block text-sm font-medium text-slate-700">Owner<select aria-label="Owner" value={filters.owner_profile_id ?? ''} onChange={(event) => updateFilters({ owner_profile_id: event.target.value, venue_id: undefined })} className="mt-1 block w-full rounded-lg border border-slate-200 px-3 py-2"><option value="">Semua owner</option>{ownerSelectionMissing && <option value={filters.owner_profile_id}>Owner terpilih</option>}{owners.map((owner) => <option key={owner.id} value={owner.id}>{owner.business_name}</option>)}</select></label><div className="mt-2 flex items-center justify-between text-xs text-slate-500"><button type="button" aria-label="Previous owner page" disabled={ownerPage <= 1} onClick={() => setOwnerPage((page) => page - 1)} className="rounded border border-slate-200 px-2 py-1 disabled:opacity-40">Sebelumnya</button><span>{ownerTotalPages > 0 ? `Halaman ${ownerPage} dari ${ownerTotalPages}` : `Halaman ${ownerPage}`}</span><button type="button" aria-label="Next owner page" disabled={ownerTotalPages > 0 && ownerPage >= ownerTotalPages} onClick={() => setOwnerPage((page) => page + 1)} className="rounded border border-slate-200 px-2 py-1 disabled:opacity-40">Berikutnya</button></div></div><label className="text-sm font-medium text-slate-700">Venue<select value={filters.venue_id ?? ''} onChange={(event) => updateFilters({ venue_id: event.target.value })} className="mt-1 block w-full rounded-lg border border-slate-200 px-3 py-2"><option value="">Semua venue</option>{venues.map((venue) => <option key={venue.id} value={venue.id}>{venue.name}</option>)}</select></label><label className="text-sm font-medium text-slate-700">Granularity<select value={filters.granularity ?? 'auto'} onChange={(event) => updateFilters({ granularity: event.target.value as PlatformFinanceSummaryQuery['granularity'] })} className="mt-1 block w-full rounded-lg border border-slate-200 px-3 py-2"><option value="auto">Otomatis</option><option value="day">Harian</option><option value="week">Mingguan</option><option value="month">Bulanan</option></select></label></div><p className="mt-3 text-xs text-slate-500">Tanggal kosong memakai periode MTD dari server dalam timezone Asia/Jakarta.</p></section>
    {optionsError && <div role="alert" data-testid="platform-finance-options-error" className="flex items-start gap-3 rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900"><AlertCircle className="mt-0.5 h-5 w-5 shrink-0" /><span>{optionsError}</span><button type="button" aria-label="Retry finance filter options" onClick={retryOptions} className="ml-auto font-semibold underline">Coba lagi</button></div>}
    {error && <div role="alert" className="flex items-start gap-3 rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800"><AlertCircle className="mt-0.5 h-5 w-5 shrink-0" /><span>{error}</span><button type="button" onClick={refresh} className="ml-auto font-semibold underline">Retry</button></div>}
    {summary && loading && <div role="status" data-testid="platform-finance-stale" className="rounded-lg border border-sky-200 bg-sky-50 p-3 text-sm text-sky-900">Memuat filter baru; ringkasan yang tampil masih berasal dari request sebelumnya.</div>}
    {summary && scopedProjectionUnavailable && <p role="status" className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900">Projected operating result belum tersedia untuk filter owner/venue karena OPEX belum memiliki alokasi per scope.</p>}
    {summary && <><section className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4"><MetricCard label="OPEX Platform" value={summary.data_availability.platform_operating_expense === 'AVAILABLE' ? formatIntegerRupiah(summary.metrics.platform_operating_expense) : 'Belum tersedia'} tone="actual" testID="platform-opex-value" hint="POSTED net exact reversal" /><MetricCard label="Proyeksi Komisi" value={formatIntegerRupiah(summary.metrics.projected_commission)} tone="projection" /><MetricCard label="Projected Operating Result" value={formatIntegerRupiah(summary.metrics.projected_operating_result_before_transaction_costs)} tone="projection" hint="Simulasi komisi dikurangi OPEX; bukan hasil aktual" /><MetricCard label="Pendapatan Aktual" value={formatActual(summary.metrics.platform_revenue)} tone="neutral" hint="Belum ada sumber kebenaran LIVE" /><MetricCard label="Transaction Contribution" value={formatActual(summary.metrics.transaction_contribution)} tone="neutral" hint="Belum ada sumber kebenaran LIVE" /><MetricCard label="Operating Result Aktual" value={formatActual(summary.metrics.operating_result)} tone="neutral" hint="Belum ada sumber kebenaran LIVE" /></section><TrendPanel summary={summary} /><section className="rounded-xl border border-slate-200 bg-white p-4 text-sm text-slate-600"><p className="font-semibold text-slate-800">Catatan data</p><ul className="mt-2 list-disc space-y-1 pl-5">{summary.caveats.map((caveat) => <li key={caveat}>{caveat}</li>)}<li>OPEX adalah biaya platform global dan tidak dialokasikan ke owner atau venue.</li></ul><p className="mt-3 text-xs text-slate-400">As of {formatCalendarDate(summary.period.end_date)} · snapshot {summary.as_of}</p></section></>}
    {!summary && loading && <div role="status" className="rounded-xl border border-slate-200 bg-white p-10 text-center text-sm text-slate-500">Memuat ringkasan finance…</div>}
  </div>;
};
