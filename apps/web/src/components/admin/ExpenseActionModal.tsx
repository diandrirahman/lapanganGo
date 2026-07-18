import React, { useEffect, useRef, useState } from 'react';
import { AlertCircle, CheckCircle2, X, XCircle } from 'lucide-react';
import { adminApi, AdminApiError } from '../../lib/api/admin';
import type { FinanceApiErrorBody, PlatformExpense } from '../../types/platformExpense';
import {
  clearExpenseAttempt,
  createExpenseAttemptState,
  getExpenseAttemptKey,
  isRetryableExpenseSubmissionError,
  validateExpenseCancelReason,
} from '../../lib/platformExpenseForm';

export type ExpenseAction = {
  type: 'cancel' | 'approve';
  expense: PlatformExpense;
};

interface ExpenseActionModalProps {
  isOpen: boolean;
  action: ExpenseAction | null;
  onClose: () => void;
  onCompleted: () => void;
  onConflict: () => void;
}

export const ExpenseActionModal: React.FC<ExpenseActionModalProps> = ({ isOpen, action, onClose, onCompleted, onConflict }) => {
  const [reason, setReason] = useState('');
  const [pending, setPending] = useState(false);
  const [errorBody, setErrorBody] = useState<FinanceApiErrorBody | null>(null);
  const [localError, setLocalError] = useState<string | null>(null);
  const attemptRef = useRef(createExpenseAttemptState());
  const identityRef = useRef<string | null>(null);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    return () => { mountedRef.current = false; };
  }, []);

  useEffect(() => {
    const nextIdentity = action ? `${action.type}:${action.expense.id}` : null;
    if (identityRef.current === nextIdentity) return;
    identityRef.current = nextIdentity;
    setReason('');
    setPending(false);
    setErrorBody(null);
    setLocalError(null);
    clearExpenseAttempt(attemptRef.current);
  }, [action]);

  if (!isOpen || !action) return null;

  const isCancel = action.type === 'cancel';
  const title = isCancel ? 'Cancel DRAFT expense' : 'Approve DRAFT expense';
  const submitLabel = isCancel ? 'Cancel expense' : 'Approve expense';
  const handleClose = () => {
    if (!attemptRef.current.key) {
      setReason('');
      setErrorBody(null);
      setLocalError(null);
      clearExpenseAttempt(attemptRef.current);
    }
    onClose();
  };

  const submit = async () => {
    if (pending) return;
    const reasonError = isCancel ? validateExpenseCancelReason(reason) : null;
    if (reasonError) {
      setLocalError(reasonError);
      return;
    }
    const payload = JSON.stringify({ expense_id: action.expense.id, ...(isCancel ? { reason: reason.trim() } : {}) });
    const idempotencyKey = getExpenseAttemptKey(attemptRef.current, payload);
    setPending(true);
    setErrorBody(null);
    setLocalError(null);
    try {
      if (isCancel) {
        await adminApi.cancelPlatformExpense(action.expense.id, reason, idempotencyKey);
      } else {
        await adminApi.approvePlatformExpense(action.expense.id, idempotencyKey);
      }
      if (!mountedRef.current) return;
      clearExpenseAttempt(attemptRef.current);
      onCompleted();
    } catch (error) {
      if (!mountedRef.current) return;
      const retryable = isRetryableExpenseSubmissionError(error);
      const body = error instanceof AdminApiError
        ? error.body
        : { message: retryable ? 'The request timed out or the connection was interrupted. Retry with the same action.' : 'Expense action could not be completed.' };
      setErrorBody(body);
      if (!retryable) clearExpenseAttempt(attemptRef.current);
      if (error instanceof AdminApiError && error.status === 409) onConflict();
    } finally {
      if (mountedRef.current) setPending(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/50 p-4" role="dialog" aria-modal="true" aria-labelledby="expense-action-title">
      <div className="w-full max-w-lg rounded-2xl bg-white shadow-2xl">
        <div className="flex items-start justify-between border-b border-slate-200 px-6 py-5">
          <div className="flex items-start gap-3">
            {isCancel ? <XCircle className="mt-0.5 h-6 w-6 text-rose-600" /> : <CheckCircle2 className="mt-0.5 h-6 w-6 text-sky-600" />}
            <div>
              <h2 id="expense-action-title" className="text-xl font-bold text-slate-900">{title}</h2>
              <p className="mt-1 text-sm text-slate-500">{action.expense.vendor || 'Unspecified vendor'} · {action.expense.description}</p>
            </div>
          </div>
          <button type="button" onClick={handleClose} disabled={pending} className="rounded-lg p-2 text-slate-400 hover:bg-slate-100 disabled:opacity-40" aria-label="Close"><X className="h-5 w-5" /></button>
        </div>
        <div className="space-y-5 px-6 py-5">
          <div className={`rounded-xl border p-4 text-sm ${isCancel ? 'border-rose-200 bg-rose-50 text-rose-900' : 'border-sky-200 bg-sky-50 text-sky-900'}`}>
            {isCancel
              ? 'This is a terminal transition. The DRAFT will not create a journal, and the reason will be retained in the audit trail.'
              : 'Approval moves this DRAFT into the accounting workflow. It does not post a journal or change P&L yet.'}
          </div>
          <dl className="grid grid-cols-2 gap-4 rounded-xl border border-slate-200 p-4 text-sm">
            <div><dt className="text-xs uppercase tracking-wide text-slate-400">Amount</dt><dd className="mt-1 font-semibold text-slate-900">Rp {action.expense.amount_rupiah}</dd></div>
            <div><dt className="text-xs uppercase tracking-wide text-slate-400">Current status</dt><dd className="mt-1 font-semibold text-slate-900">{action.expense.status}</dd></div>
          </dl>
          {isCancel && <label className="block text-sm font-medium text-slate-700">Reason
            <textarea value={reason} onChange={(event) => { setReason(event.target.value); setLocalError(null); setErrorBody(null); }} maxLength={500} rows={4} className="mt-1 block w-full rounded-lg border border-slate-200 px-3 py-2" placeholder="Why is this expense being cancelled?" />
            <span className="mt-1 block text-xs text-slate-400">{reason.length}/500 characters</span>
          </label>}
          {localError && <p className="text-sm text-rose-700">{localError}</p>}
          {errorBody && <div className="flex items-start gap-2 rounded-lg bg-rose-50 p-3 text-sm text-rose-800" role="alert"><AlertCircle className="mt-0.5 h-4 w-4 shrink-0" /><span>{errorBody.message || 'The action could not be completed.'}</span></div>}
          <div className="flex justify-end gap-3"><button type="button" onClick={handleClose} disabled={pending} className="rounded-lg border border-slate-200 px-4 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50 disabled:opacity-50">Keep as DRAFT</button><button type="button" onClick={() => void submit()} disabled={pending} className={`rounded-lg px-4 py-2 text-sm font-semibold text-white disabled:cursor-not-allowed disabled:opacity-50 ${isCancel ? 'bg-rose-600 hover:bg-rose-700' : 'bg-sky-600 hover:bg-sky-700'}`}>{pending ? 'Submitting…' : submitLabel}</button></div>
        </div>
      </div>
    </div>
  );
};
