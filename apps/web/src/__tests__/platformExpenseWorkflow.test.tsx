import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ExpenseActionModal } from '../components/admin/ExpenseActionModal';
import { AdminPlatformExpensesPage } from '../pages/admin/AdminPlatformExpensesPage';
import { AdminApiError, adminApi } from '../lib/api/admin';
import type { PlatformExpense, PlatformJournal } from '../types/platformExpense';

vi.mock('../lib/api/admin', async () => {
  const actual = await vi.importActual<typeof import('../lib/api/admin')>('../lib/api/admin');
  return {
    ...actual,
    adminApi: {
      ...actual.adminApi,
      getPlatformExpenses: vi.fn(),
      getPlatformJournals: vi.fn(),
      postPlatformExpense: vi.fn(),
      voidPlatformExpense: vi.fn(),
    },
  };
});

const expense: PlatformExpense = {
  id: 'expense-1',
  category: 'OFFICE_ADMIN',
  vendor: 'QA Vendor',
  amount_rupiah: '125000',
  currency: 'IDR',
  occurred_at: '2026-07-17T13:00:00Z',
  payment_account: 'FUNDING_CLEARING',
  external_reference: 'QA-001',
  description: 'Executable post and void QA',
  status: 'APPROVED',
  posted_journal_id: null,
  void_journal_id: null,
  created_by_user_id: 'user-1',
  approved_by_user_id: 'user-1',
  posted_by_user_id: null,
  voided_by_user_id: null,
  cancelled_by_user_id: null,
  cancel_reason: null,
  void_reason: null,
  created_at: '2026-07-17T13:00:00Z',
  approved_at: '2026-07-17T13:01:00Z',
  posted_at: null,
  voided_at: null,
  cancelled_at: null,
};

const postedExpense: PlatformExpense = {
  ...expense,
  status: 'POSTED',
  posted_journal_id: 'journal-posted-1',
  posted_by_user_id: 'user-1',
  posted_at: '2026-07-17T13:02:00Z',
};

const reversalExpense: PlatformExpense = {
  ...postedExpense,
  status: 'VOID',
  void_journal_id: 'journal-reversal-1',
  voided_by_user_id: 'user-1',
  voided_at: '2026-07-17T13:03:00Z',
  void_reason: 'supplier correction',
};

const journal: PlatformJournal = {
  id: 'journal-reversal-1',
  event_key: 'journal.reversed:journal-posted-1',
  event_type: 'PLATFORM_FINANCE_JOURNAL_REVERSED',
  booking_id: null,
  owner_profile_id: null,
  venue_id: null,
  currency: 'IDR',
  effective_at: '2026-07-17T13:03:00Z',
  posted_at: '2026-07-17T13:03:00Z',
  reverses_journal_id: 'journal-posted-1',
  reversal_reason: 'supplier correction',
  reversed_by_journal_id: null,
  entry_count: 2,
  debit_total_rupiah: '125000',
  credit_total_rupiah: '125000',
};

const expensePage = (item: PlatformExpense) => ({
  data: [item], page: 1, limit: 20, total_items: 1, total_pages: 1,
});

const journalPage = {
  data: [journal], page: 1, limit: 20, total_items: 1, total_pages: 1,
};

const postPlatformExpense = vi.mocked(adminApi.postPlatformExpense);
const voidPlatformExpense = vi.mocked(adminApi.voidPlatformExpense);
const getPlatformExpenses = vi.mocked(adminApi.getPlatformExpenses);
const getPlatformJournals = vi.mocked(adminApi.getPlatformJournals);

const action = { type: 'post' as const, expense };
const voidAction = { type: 'void' as const, expense: postedExpense };

beforeEach(() => {
  vi.clearAllMocks();
});

afterEach(() => {
  cleanup();
});

describe('ExpenseActionModal executable mutation QA', () => {
  it('sends exactly one post request when the action is double-clicked', async () => {
    const user = userEvent.setup();
    const onCompleted = vi.fn();
    let resolvePost!: (value: PlatformExpense) => void;
    postPlatformExpense.mockImplementation(() => new Promise((resolve) => { resolvePost = resolve; }));

    render(<ExpenseActionModal isOpen action={action} onClose={vi.fn()} onCompleted={onCompleted} onConflict={vi.fn()} />);
    const submit = screen.getByRole('button', { name: 'Post expense' });

    await user.dblClick(submit);

    expect(postPlatformExpense).toHaveBeenCalledTimes(1);
    expect(postPlatformExpense.mock.calls[0]?.[0]).toBe(expense.id);
    expect(postPlatformExpense.mock.calls[0]?.[1]).toBeTruthy();

    resolvePost(postedExpense);
    await waitFor(() => expect(onCompleted).toHaveBeenCalledTimes(1));
  });

  it('keeps the same key after timeout, close, reopen, and retry', async () => {
    const user = userEvent.setup();
    postPlatformExpense
      .mockRejectedValueOnce(new Error('Request timeout'))
      .mockResolvedValueOnce(postedExpense);
    const onClose = vi.fn();
    const onCompleted = vi.fn();
    const { rerender } = render(<ExpenseActionModal isOpen action={action} onClose={onClose} onCompleted={onCompleted} onConflict={vi.fn()} />);

    await user.click(screen.getByRole('button', { name: 'Post expense' }));
    await screen.findByRole('alert');
    const firstKey = postPlatformExpense.mock.calls[0]?.[1];
    expect(firstKey).toBeTruthy();

    await user.click(screen.getByRole('button', { name: 'Close' }));
    expect(onClose).toHaveBeenCalledTimes(1);
    rerender(<ExpenseActionModal isOpen={false} action={action} onClose={onClose} onCompleted={onCompleted} onConflict={vi.fn()} />);
    rerender(<ExpenseActionModal isOpen action={action} onClose={onClose} onCompleted={onCompleted} onConflict={vi.fn()} />);

    await user.click(screen.getByRole('button', { name: 'Post expense' }));
    await waitFor(() => expect(postPlatformExpense).toHaveBeenCalledTimes(2));
    expect(postPlatformExpense.mock.calls[1]?.[1]).toBe(firstKey);
    await waitFor(() => expect(onCompleted).toHaveBeenCalledTimes(1));
  });

  it('requires a reason and sends exactly one void request on double-click', async () => {
    const user = userEvent.setup();
    const onCompleted = vi.fn();
    let resolveVoid!: (value: PlatformExpense) => void;
    voidPlatformExpense.mockImplementation(() => new Promise((resolve) => { resolveVoid = resolve; }));

    render(<ExpenseActionModal isOpen action={voidAction} onClose={vi.fn()} onCompleted={onCompleted} onConflict={vi.fn()} />);
    const submit = screen.getByRole('button', { name: 'Void with reversal' });
    await user.dblClick(submit);
    expect(voidPlatformExpense).not.toHaveBeenCalled();
    expect(screen.getByText('A void reason is required.')).toBeTruthy();

    await user.type(screen.getByPlaceholderText('Why is this expense being voided?'), 'supplier correction');
    await user.dblClick(submit);
    expect(voidPlatformExpense).toHaveBeenCalledTimes(1);
    expect(voidPlatformExpense.mock.calls[0]?.[0]).toBe(postedExpense.id);
    expect(voidPlatformExpense.mock.calls[0]?.[1]).toBe('supplier correction');
    expect(voidPlatformExpense.mock.calls[0]?.[2]).toBeTruthy();

    resolveVoid(reversalExpense);
    await waitFor(() => expect(onCompleted).toHaveBeenCalledTimes(1));
  });

  it('keeps the void key after timeout, close, reopen, and retry', async () => {
    const user = userEvent.setup();
    voidPlatformExpense
      .mockRejectedValueOnce(new Error('Request timeout'))
      .mockResolvedValueOnce(reversalExpense);
    const onClose = vi.fn();
    const onCompleted = vi.fn();
    const { rerender } = render(<ExpenseActionModal isOpen action={voidAction} onClose={onClose} onCompleted={onCompleted} onConflict={vi.fn()} />);

    await user.type(screen.getByPlaceholderText('Why is this expense being voided?'), 'supplier correction');
    await user.click(screen.getByRole('button', { name: 'Void with reversal' }));
    await screen.findByRole('alert');
    const firstKey = voidPlatformExpense.mock.calls[0]?.[2];
    expect(firstKey).toBeTruthy();

    await user.click(screen.getByRole('button', { name: 'Close' }));
    expect(onClose).toHaveBeenCalledTimes(1);
    rerender(<ExpenseActionModal isOpen={false} action={voidAction} onClose={onClose} onCompleted={onCompleted} onConflict={vi.fn()} />);
    rerender(<ExpenseActionModal isOpen action={voidAction} onClose={onClose} onCompleted={onCompleted} onConflict={vi.fn()} />);

    await user.click(screen.getByRole('button', { name: 'Void with reversal' }));
    await waitFor(() => expect(voidPlatformExpense).toHaveBeenCalledTimes(2));
    expect(voidPlatformExpense.mock.calls[1]?.[2]).toBe(firstKey);
    await waitFor(() => expect(onCompleted).toHaveBeenCalledTimes(1));
  });
});

describe('AdminPlatformExpensesPage executable refresh and link QA', () => {
  it('refetches and renders the new status after a successful post', async () => {
    const user = userEvent.setup();
    getPlatformExpenses
      .mockResolvedValueOnce(expensePage(expense))
      .mockResolvedValueOnce(expensePage(postedExpense));
    postPlatformExpense.mockResolvedValue(postedExpense);

    render(<AdminPlatformExpensesPage />);
    await screen.findAllByText('APPROVED');
    const postButtons = screen.getAllByRole('button', { name: /^Post$/ });
    expect(postButtons).toHaveLength(2);
    await user.click(postButtons[0]);
    await user.click(screen.getByRole('button', { name: 'Post expense' }));

    await waitFor(() => expect(getPlatformExpenses).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(screen.getAllByText('POSTED').length).toBeGreaterThan(0));
    expect(postPlatformExpense).toHaveBeenCalledTimes(1);
  });

  it('refetches after a 409 conflict and keeps the authoritative status', async () => {
    const user = userEvent.setup();
    getPlatformExpenses
      .mockResolvedValueOnce(expensePage(expense))
      .mockResolvedValueOnce(expensePage(expense));
    postPlatformExpense.mockRejectedValue(new AdminApiError(409, { code: 'CONFLICT', message: 'Already posted' }, 'Conflict'));

    render(<AdminPlatformExpensesPage />);
    await screen.findAllByText('APPROVED');
    const postButtons = screen.getAllByRole('button', { name: /^Post$/ });
    await user.click(postButtons[0]);
    await user.click(screen.getByRole('button', { name: 'Post expense' }));

    await waitFor(() => expect(getPlatformExpenses).toHaveBeenCalledTimes(2));
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(screen.getAllByText('APPROVED').length).toBeGreaterThan(0);
  });

  it('focuses the exact reversal journal through journal_id after clicking the link', async () => {
    const user = userEvent.setup();
    getPlatformExpenses.mockResolvedValue(expensePage(reversalExpense));
    getPlatformJournals.mockResolvedValue(journalPage);

    const { container } = render(<AdminPlatformExpensesPage />);
    await screen.findAllByText('VOID');
    const reversalLinks = container.querySelectorAll('a[href="#journal-journal-reversal-1"]');
    expect(reversalLinks).toHaveLength(2);
    await user.click(reversalLinks[0] as HTMLAnchorElement);

    await waitFor(() => expect(getPlatformJournals).toHaveBeenCalled());
    expect(getPlatformJournals.mock.calls.at(-1)?.[0]).toMatchObject({ journal_id: journal.id, page: 1 });
    await screen.findByText(/Showing linked journal/);
  });
});
