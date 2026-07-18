import React, { useCallback, useEffect, useRef, useState } from 'react';
import { AlertCircle, BookOpen, CheckCircle2, ChevronLeft, ChevronRight, CircleDollarSign, Plus, RefreshCw, RotateCcw, XCircle } from 'lucide-react';
import { format } from 'date-fns';
import { adminApi } from '../../lib/api/admin';
import type { PlatformExpensePage, PlatformJournalPage } from '../../lib/api/admin';
import type { ExpenseCategory, ExpenseStatus, PlatformExpense, PlatformJournal } from '../../types/platformExpense';
import { EXPENSE_CATEGORIES, EXPENSE_STATUSES } from '../../types/platformExpense';
import { CreateExpenseModal } from '../../components/admin/CreateExpenseModal';
import { ExpenseActionModal, type ExpenseAction } from '../../components/admin/ExpenseActionModal';
import { createExpenseMutationRefreshState, createJournalFocusState } from '../../lib/platformExpenseWorkflow';

const PAGE_LIMIT = 20;

const isAbortError = (error: unknown): boolean => (
  error instanceof DOMException && error.name === 'AbortError'
  || error instanceof Error && error.name === 'AbortError'
);

const shortID = (value: string | null): string => value ? (value.length > 14 ? `${value.slice(0, 8)}…${value.slice(-4)}` : value) : '—';

const formatDateTime = (value: string | null): string => {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '—' : format(date, 'dd MMM yyyy, HH:mm');
};

const formatRupiah = (value: string): string => {
  try { return `Rp ${BigInt(value).toLocaleString('id-ID')}`; } catch { return 'Rp —'; }
};

const statusStyle: Record<ExpenseStatus, string> = {
  DRAFT: 'bg-slate-100 text-slate-700',
  APPROVED: 'bg-sky-100 text-sky-800',
  POSTED: 'bg-emerald-100 text-emerald-800',
  VOID: 'bg-amber-100 text-amber-800',
  CANCELLED: 'bg-rose-100 text-rose-800',
};

const StatusBadge: React.FC<{ status: ExpenseStatus }> = ({ status }) => (
  <span className={`inline-flex rounded-full px-2.5 py-1 text-xs font-semibold ${statusStyle[status]}`}>{status}</span>
);

const ExpenseList: React.FC<{ expenses: PlatformExpense[]; onAction: (action: ExpenseAction) => void; onJournalLink: (journalID: string) => void }> = ({ expenses, onAction, onJournalLink }) => (
  <div onClick={(event) => {
    const anchor = (event.target as HTMLElement).closest('a[href^="#journal-"]') as HTMLAnchorElement | null;
    const href = anchor?.getAttribute('href');
    if (!href) return;
    event.preventDefault();
    onJournalLink(href.slice('#journal-'.length));
  }}>
    <div className="hidden overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm md:block">
      <div className="overflow-x-auto">
        <table className="min-w-full divide-y divide-slate-200">
          <thead className="bg-slate-50"><tr>
            {['Expense', 'Amount', 'When', 'Status', 'Journal link', 'Actions'].map((heading) => <th key={heading} className="px-5 py-3 text-left text-xs font-semibold uppercase tracking-wide text-slate-500">{heading}</th>)}
          </tr></thead>
          <tbody className="divide-y divide-slate-100">
            {expenses.map((expense) => (
              <tr key={expense.id} className="hover:bg-slate-50">
                <td className="px-5 py-4 align-top"><p className="font-semibold text-slate-900">{expense.vendor || 'Unspecified vendor'}</p><p className="mt-1 text-sm text-slate-700">{expense.description}</p><p className="mt-1 text-xs uppercase tracking-wide text-slate-400">{expense.category} · {shortID(expense.id)}</p></td>
                <td className="px-5 py-4 align-top text-sm font-semibold text-slate-900">{formatRupiah(expense.amount_rupiah)}<p className="mt-1 text-xs font-normal text-slate-500">{expense.payment_account}</p></td>
                <td className="px-5 py-4 align-top text-sm text-slate-700">{formatDateTime(expense.occurred_at)}<p className="mt-1 text-xs text-slate-400">Created {formatDateTime(expense.created_at)}</p></td>
                <td className="px-5 py-4 align-top"><StatusBadge status={expense.status} /></td>
                <td className="px-5 py-4 align-top text-sm text-slate-700">{expense.posted_journal_id ? <a href={`#journal-${expense.posted_journal_id}`} onClick={(event) => { event.preventDefault(); onJournalLink(expense.posted_journal_id as string); }} className="font-medium text-emerald-700 hover:underline">Posted {shortID(expense.posted_journal_id)}</a> : <span className="text-slate-400">No journal yet</span>}{expense.void_journal_id && <p className="mt-1 text-xs text-amber-700"><a href={`#journal-${expense.void_journal_id}`} onClick={(event) => { event.preventDefault(); onJournalLink(expense.void_journal_id as string); }} className="hover:underline">Void reversal {shortID(expense.void_journal_id)}</a></p>}</td>
                <td className="px-5 py-4 align-top">{expense.status === 'DRAFT' ? <div className="flex flex-col items-start gap-2"><button type="button" onClick={() => onAction({ type: 'approve', expense })} className="inline-flex items-center rounded-lg bg-sky-600 px-3 py-1.5 text-xs font-semibold text-white hover:bg-sky-700"><CheckCircle2 className="mr-1.5 h-3.5 w-3.5" />Approve</button><button type="button" onClick={() => onAction({ type: 'cancel', expense })} className="inline-flex items-center rounded-lg border border-rose-200 px-3 py-1.5 text-xs font-semibold text-rose-700 hover:bg-rose-50"><XCircle className="mr-1.5 h-3.5 w-3.5" />Cancel</button></div> : expense.status === 'APPROVED' ? <button type="button" onClick={() => onAction({ type: 'post', expense })} className="inline-flex items-center rounded-lg bg-emerald-600 px-3 py-1.5 text-xs font-semibold text-white hover:bg-emerald-700"><CircleDollarSign className="mr-1.5 h-3.5 w-3.5" />Post</button> : expense.status === 'POSTED' ? <button type="button" onClick={() => onAction({ type: 'void', expense })} className="inline-flex items-center rounded-lg border border-rose-200 px-3 py-1.5 text-xs font-semibold text-rose-700 hover:bg-rose-50"><XCircle className="mr-1.5 h-3.5 w-3.5" />Void</button> : <span className="text-xs text-slate-400">No actions</span>}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
    <div className="space-y-3 md:hidden">{expenses.map((expense) => <article key={expense.id} className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm"><div className="flex items-start justify-between gap-3"><div><h2 className="font-semibold text-slate-900">{expense.vendor || 'Unspecified vendor'}</h2><p className="mt-1 text-sm text-slate-700">{expense.description}</p></div><StatusBadge status={expense.status} /></div><dl className="mt-4 grid grid-cols-2 gap-3 text-sm"><div><dt className="text-xs uppercase tracking-wide text-slate-400">Amount</dt><dd className="font-semibold text-slate-900">{formatRupiah(expense.amount_rupiah)}</dd></div><div><dt className="text-xs uppercase tracking-wide text-slate-400">Occurred</dt><dd className="text-slate-700">{formatDateTime(expense.occurred_at)}</dd></div></dl><p className="mt-3 text-xs text-slate-500">{expense.posted_journal_id ? <a href={`#journal-${expense.posted_journal_id}`} className="text-emerald-700 hover:underline">Posted journal {shortID(expense.posted_journal_id)}</a> : 'No journal yet'}{expense.void_journal_id && <>{' · '}<a href={`#journal-${expense.void_journal_id}`} className="text-amber-700 hover:underline">Void reversal {shortID(expense.void_journal_id)}</a></>} · {expense.category}</p>{expense.status === 'DRAFT' ? <div className="mt-4 flex gap-2"><button type="button" onClick={() => onAction({ type: 'approve', expense })} className="inline-flex flex-1 items-center justify-center rounded-lg bg-sky-600 px-3 py-2 text-xs font-semibold text-white hover:bg-sky-700"><CheckCircle2 className="mr-1.5 h-3.5 w-3.5" />Approve</button><button type="button" onClick={() => onAction({ type: 'cancel', expense })} className="inline-flex flex-1 items-center justify-center rounded-lg border border-rose-200 px-3 py-2 text-xs font-semibold text-rose-700 hover:bg-rose-50"><XCircle className="mr-1.5 h-3.5 w-3.5" />Cancel</button></div> : expense.status === 'APPROVED' ? <button type="button" onClick={() => onAction({ type: 'post', expense })} className="mt-4 inline-flex w-full items-center justify-center rounded-lg bg-emerald-600 px-3 py-2 text-xs font-semibold text-white hover:bg-emerald-700"><CircleDollarSign className="mr-1.5 h-3.5 w-3.5" />Post</button> : expense.status === 'POSTED' ? <button type="button" onClick={() => onAction({ type: 'void', expense })} className="mt-4 inline-flex w-full items-center justify-center rounded-lg border border-rose-200 px-3 py-2 text-xs font-semibold text-rose-700 hover:bg-rose-50"><XCircle className="mr-1.5 h-3.5 w-3.5" />Void with reversal</button> : null}</article>)}</div>
  </div>
);

const JournalList: React.FC<{ journals: PlatformJournal[]; selectedJournalID: string; onJournalLink: (journalID: string) => void }> = ({ journals, selectedJournalID, onJournalLink }) => (
  <div data-focused-journal={selectedJournalID || undefined} onClick={(event) => {
    const anchor = (event.target as HTMLElement).closest('a[href^="#journal-"]') as HTMLAnchorElement | null;
    const href = anchor?.getAttribute('href');
    if (!href) return;
    event.preventDefault();
    onJournalLink(href.slice('#journal-'.length));
  }}>
    {selectedJournalID && <p className="mb-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900" role="status">Showing linked journal {shortID(selectedJournalID)}.</p>}
    <div className="hidden overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm md:block"><div className="overflow-x-auto"><table className="min-w-full divide-y divide-slate-200"><thead className="bg-slate-50"><tr>{['Journal', 'Event', 'Effective', 'Balanced total', 'Reversal'].map((heading) => <th key={heading} className="px-5 py-3 text-left text-xs font-semibold uppercase tracking-wide text-slate-500">{heading}</th>)}</tr></thead><tbody className="divide-y divide-slate-100">{journals.map((journal) => <tr id={`journal-${journal.id}`} key={journal.id} className="hover:bg-slate-50"><td className="px-5 py-4 align-top"><p className="font-semibold text-slate-900">{shortID(journal.id)}</p><p className="mt-1 text-xs text-slate-500">{journal.entry_count} entries</p></td><td className="px-5 py-4 align-top"><p className="font-medium text-slate-800">{journal.event_type}</p><p className="mt-1 text-xs text-slate-500">{journal.event_key}</p></td><td className="px-5 py-4 align-top text-sm text-slate-700">{formatDateTime(journal.effective_at)}<p className="mt-1 text-xs text-slate-400">Posted {formatDateTime(journal.posted_at)}</p></td><td className="px-5 py-4 align-top text-sm font-semibold text-slate-900">{formatRupiah(journal.debit_total_rupiah)}<p className="mt-1 text-xs font-normal text-emerald-700">Debit = credit</p></td><td className="px-5 py-4 align-top text-sm">{journal.reverses_journal_id ? <a href={`#journal-${journal.reverses_journal_id}`} className="text-amber-700 hover:underline">Reversal of {shortID(journal.reverses_journal_id)}</a> : journal.reversed_by_journal_id ? <a href={`#journal-${journal.reversed_by_journal_id}`} className="text-amber-700 hover:underline">Reversed by {shortID(journal.reversed_by_journal_id)}</a> : <span className="text-slate-400">—</span>}</td></tr>)}</tbody></table></div></div>
    <div className="space-y-3 md:hidden">{journals.map((journal) => <article id={`journal-${journal.id}`} key={journal.id} className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm"><div className="flex items-start justify-between"><div><h2 className="font-semibold text-slate-900">{journal.event_type}</h2><p className="mt-1 text-xs text-slate-500">{shortID(journal.id)} · {journal.entry_count} entries</p></div><span className="text-xs font-semibold text-emerald-700">Balanced</span></div><p className="mt-3 text-sm text-slate-700">{formatDateTime(journal.effective_at)} · {formatRupiah(journal.debit_total_rupiah)}</p>{journal.reverses_journal_id && <p className="mt-2 text-xs text-amber-700">Reverses {shortID(journal.reverses_journal_id)}</p>}</article>)}</div>
  </div>
);

export const AdminPlatformExpensesPage: React.FC = () => {
  const [tab, setTab] = useState<'expenses' | 'journals'>('expenses');
  const [status, setStatus] = useState<ExpenseStatus | ''>('');
  const [category, setCategory] = useState<ExpenseCategory | ''>('');
  const [startDate, setStartDate] = useState('');
  const [endDate, setEndDate] = useState('');
  const [eventType, setEventType] = useState('');
  const [journalID, setJournalID] = useState('');
  const [page, setPage] = useState(1);
  const [expensePage, setExpensePage] = useState<PlatformExpensePage | null>(null);
  const [journalPage, setJournalPage] = useState<PlatformJournalPage | null>(null);
  const [refreshToken, setRefreshToken] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [actionDraft, setActionDraft] = useState<ExpenseAction | null>(null);
  const [isActionOpen, setIsActionOpen] = useState(false);
  const controllerRef = useRef<AbortController | null>(null);
  const requestIDRef = useRef(0);

  const fetchData = useCallback(async () => {
    controllerRef.current?.abort();
    const controller = new AbortController();
    controllerRef.current = controller;
    const requestID = ++requestIDRef.current;
    setLoading(true); setError(null);
    try {
      if (tab === 'expenses') {
        const response = await adminApi.getPlatformExpenses({ page, limit: PAGE_LIMIT, ...(status ? { status } : {}), ...(category ? { category } : {}) }, { signal: controller.signal });
        if (requestID !== requestIDRef.current) return;
        setExpensePage(response);
      } else {
        const response = await adminApi.getPlatformJournals({ page, limit: PAGE_LIMIT, ...(journalID ? { journal_id: journalID } : {}), ...(startDate ? { start_date: startDate } : {}), ...(endDate ? { end_date: endDate } : {}), ...(eventType.trim() ? { event_type: eventType.trim().toUpperCase() } : {}) }, { signal: controller.signal });
        if (requestID !== requestIDRef.current) return;
        setJournalPage(response);
      }
    } catch (requestError) {
      if (requestID !== requestIDRef.current || isAbortError(requestError)) return;
      setError('Finance data could not be loaded. Please try again.');
    } finally {
      if (requestID === requestIDRef.current) setLoading(false);
    }
  }, [category, endDate, eventType, journalID, page, startDate, status, tab]);

  useEffect(() => { void fetchData(); return () => controllerRef.current?.abort(); }, [fetchData, refreshToken]);
  const totalPages = tab === 'expenses' ? expensePage?.total_pages ?? 0 : journalPage?.total_pages ?? 0;

  const resetFilters = () => { setStatus(''); setCategory(''); setStartDate(''); setEndDate(''); setEventType(''); setJournalID(''); setPage(1); };
  const switchTab = (nextTab: 'expenses' | 'journals') => { setTab(nextTab); setJournalID(nextTab === 'journals' ? journalID : ''); setPage(1); setError(null); };
  const openJournal = (targetJournalID: string) => { const focus = createJournalFocusState(targetJournalID); setTab(focus.tab); setJournalID(focus.journalID); if (focus.clearDateFilters) { setStartDate(''); setEndDate(''); setEventType(''); } setPage(focus.page); setError(null); };
  const refreshExpenses = () => { const refresh = createExpenseMutationRefreshState(); setTab(refresh.tab); setPage(refresh.page); if (refresh.clearJournalFocus) setJournalID(''); if (refresh.incrementRefreshToken) setRefreshToken((current) => current + 1); setError(null); };
  const openAction = (action: ExpenseAction) => { setActionDraft(action); setIsActionOpen(true); };

  return <div className="mx-auto max-w-7xl space-y-6">
    <header className="flex flex-col justify-between gap-4 sm:flex-row sm:items-start"><div className="flex items-center gap-3"><div className="flex h-11 w-11 items-center justify-center rounded-xl bg-emerald-100 text-emerald-700"><CircleDollarSign className="h-6 w-6" /></div><div><h1 className="text-2xl font-bold text-slate-900">Platform Finance</h1><p className="mt-1 text-sm text-slate-500">LapangGo operating expenses and immutable journal evidence.</p></div></div><div className="flex gap-2"><button type="button" onClick={() => setIsCreateOpen(true)} className="inline-flex items-center justify-center rounded-lg bg-emerald-600 px-4 py-2 text-sm font-semibold text-white shadow-sm hover:bg-emerald-700"><Plus className="mr-2 h-4 w-4" />Add expense</button><button type="button" onClick={() => void fetchData()} disabled={loading} className="inline-flex items-center justify-center rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-60"><RefreshCw className={`mr-2 h-4 w-4 ${loading ? 'animate-spin' : ''}`} />Refresh</button></div></header>
    <div className="flex gap-2 border-b border-slate-200"><button type="button" onClick={() => switchTab('expenses')} className={`inline-flex items-center border-b-2 px-3 py-3 text-sm font-semibold ${tab === 'expenses' ? 'border-emerald-600 text-emerald-700' : 'border-transparent text-slate-500'}`}><CircleDollarSign className="mr-2 h-4 w-4" />Expenses</button><button type="button" onClick={() => switchTab('journals')} className={`inline-flex items-center border-b-2 px-3 py-3 text-sm font-semibold ${tab === 'journals' ? 'border-emerald-600 text-emerald-700' : 'border-transparent text-slate-500'}`}><BookOpen className="mr-2 h-4 w-4" />Journals</button></div>
    <section className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm"><div className="grid gap-3 md:grid-cols-4">{tab === 'expenses' ? <><label className="text-sm font-medium text-slate-700">Status<select value={status} onChange={(event) => { setStatus(event.target.value as ExpenseStatus | ''); setPage(1); }} className="mt-1 block w-full rounded-lg border border-slate-200 px-3 py-2"><option value="">All statuses</option>{EXPENSE_STATUSES.map((item) => <option key={item} value={item}>{item}</option>)}</select></label><label className="text-sm font-medium text-slate-700">Category<select value={category} onChange={(event) => { setCategory(event.target.value as ExpenseCategory | ''); setPage(1); }} className="mt-1 block w-full rounded-lg border border-slate-200 px-3 py-2"><option value="">All categories</option>{EXPENSE_CATEGORIES.map((item) => <option key={item} value={item}>{item}</option>)}</select></label></> : <><label className="text-sm font-medium text-slate-700">Start date<input type="date" value={startDate} onChange={(event) => { setStartDate(event.target.value); setPage(1); }} className="mt-1 block w-full rounded-lg border border-slate-200 px-3 py-2" /></label><label className="text-sm font-medium text-slate-700">End date<input type="date" value={endDate} onChange={(event) => { setEndDate(event.target.value); setPage(1); }} className="mt-1 block w-full rounded-lg border border-slate-200 px-3 py-2" /></label><label className="text-sm font-medium text-slate-700 md:col-span-2">Event type<input value={eventType} onChange={(event) => { setEventType(event.target.value); setPage(1); }} placeholder="e.g. EXPENSE_POSTED" className="mt-1 block w-full rounded-lg border border-slate-200 px-3 py-2" /></label></>}<button type="button" onClick={resetFilters} className="inline-flex items-center self-end justify-center rounded-lg border border-slate-200 px-3 py-2 text-sm font-medium text-slate-600 hover:bg-slate-50"><RotateCcw className="mr-2 h-4 w-4" />Reset filters</button></div></section>
    {error && <div className="flex items-start gap-3 rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800" role="alert"><AlertCircle className="mt-0.5 h-5 w-5 shrink-0" /><div className="flex-1">{error}</div><button type="button" onClick={() => void fetchData()} className="font-semibold underline">Retry</button></div>}
    {loading ? <div className="space-y-3" role="status" aria-label="Loading platform finance"><div className="h-40 animate-pulse rounded-xl border border-slate-200 bg-white" /><div className="h-40 animate-pulse rounded-xl border border-slate-200 bg-white" /></div> : tab === 'expenses' ? expensePage && expensePage.data.length > 0 ? <ExpenseList expenses={expensePage.data} onAction={openAction} onJournalLink={openJournal} /> : <div className="rounded-xl border border-dashed border-slate-300 bg-white p-10 text-center text-sm text-slate-500">No platform expenses match the current filters.</div> : journalPage && journalPage.data.length > 0 ? <JournalList journals={journalPage.data} selectedJournalID={journalID} onJournalLink={openJournal} /> : <div className="rounded-xl border border-dashed border-slate-300 bg-white p-10 text-center text-sm text-slate-500">No posted journals match the current filters.</div>}
    {!loading && totalPages > 0 && <div className="flex items-center justify-between rounded-xl border border-slate-200 bg-white px-4 py-3 text-sm text-slate-600"><span>Page {page} of {totalPages}</span><div className="flex gap-2"><button type="button" onClick={() => setPage((current) => Math.max(1, current - 1))} disabled={page <= 1} className="rounded-lg border border-slate-200 p-2 disabled:opacity-40"><ChevronLeft className="h-4 w-4" /></button><button type="button" onClick={() => setPage((current) => Math.min(totalPages, current + 1))} disabled={page >= totalPages} className="rounded-lg border border-slate-200 p-2 disabled:opacity-40"><ChevronRight className="h-4 w-4" /></button></div></div>}
    <CreateExpenseModal isOpen={isCreateOpen} onClose={() => setIsCreateOpen(false)} onCreated={() => { setIsCreateOpen(false); refreshExpenses(); }} />
    <ExpenseActionModal isOpen={isActionOpen} action={actionDraft} onClose={() => setIsActionOpen(false)} onCompleted={() => { setIsActionOpen(false); setActionDraft(null); refreshExpenses(); }} onConflict={() => { setIsActionOpen(false); setActionDraft(null); refreshExpenses(); }} />
  </div>;
};
