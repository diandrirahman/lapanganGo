import React, { useEffect, useMemo, useRef, useState } from 'react';
import { AlertCircle, CheckCircle2, X } from 'lucide-react';
import { adminApi, AdminApiError } from '../../lib/api/admin';
import type {
  CreatePlatformExpenseRequest,
  ExpenseCategory,
  ExpensePaymentAccount,
  FinanceApiErrorBody,
} from '../../types/platformExpense';
import { EXPENSE_CATEGORIES, EXPENSE_PAYMENT_ACCOUNTS } from '../../types/platformExpense';
import {
  clearExpenseAttempt,
  createExpenseAttemptState,
  formatJakartaDateTimeInput,
  getExpenseAttemptKey,
  isRetryableExpenseSubmissionError,
  toJakartaExpenseTimestamp,
  validateCreateExpenseForm,
} from '../../lib/platformExpenseForm';

interface CreateExpenseModalProps {
  isOpen: boolean;
  onClose: () => void;
  onCreated: () => void;
}

type FormState = {
  amount_rupiah: string;
  occurred_at: string;
  category: ExpenseCategory;
  payment_account: ExpensePaymentAccount;
  vendor: string;
  external_reference: string;
  description: string;
};

const initialForm = (): FormState => ({
  amount_rupiah: '',
  occurred_at: formatJakartaDateTimeInput(),
  category: 'INFRASTRUCTURE',
  payment_account: 'FUNDING_CLEARING',
  vendor: '',
  external_reference: '',
  description: '',
});

const toRequest = (form: FormState): CreatePlatformExpenseRequest => ({
  amount_rupiah: form.amount_rupiah.trim(),
  currency: 'IDR',
  occurred_at: /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(form.occurred_at)
    ? toJakartaExpenseTimestamp(form.occurred_at)
    : '',
  category: form.category,
  payment_account: form.payment_account,
  ...(form.vendor.trim() ? { vendor: form.vendor.trim() } : {}),
  ...(form.external_reference.trim() ? { external_reference: form.external_reference.trim() } : {}),
  description: form.description.trim(),
});

const formatRupiah = (value: string): string => {
  try {
    return `Rp ${BigInt(value).toLocaleString('id-ID')}`;
  } catch {
    return 'Rp —';
  }
};

const ErrorFields: React.FC<{ body: FinanceApiErrorBody | null; localErrors: Record<string, string> }> = ({ body, localErrors }) => {
  const fields = { ...localErrors, ...(body?.field_errors ?? {}) };
  if (Object.keys(fields).length === 0) return null;
  return (
    <div className="space-y-1 text-sm text-rose-700">
      {Object.entries(fields).map(([key, message]) => (
        <p key={key}><span className="font-semibold">{key.replaceAll('_', ' ')}:</span> {message}</p>
      ))}
    </div>
  );
};

export const CreateExpenseModal: React.FC<CreateExpenseModalProps> = ({ isOpen, onClose, onCreated }) => {
  const [form, setForm] = useState<FormState>(initialForm);
  const [step, setStep] = useState<'form' | 'confirm'>('form');
  const [pending, setPending] = useState(false);
  const [errorBody, setErrorBody] = useState<FinanceApiErrorBody | null>(null);
  const [localErrors, setLocalErrors] = useState<Record<string, string>>({});
  const attemptRef = useRef(createExpenseAttemptState());
  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => { mountedRef.current = false; };
  }, []);

  const request = useMemo(() => toRequest(form), [form]);
  const requestPayload = useMemo(() => JSON.stringify(request), [request]);
  const amountLabel = formatRupiah(form.amount_rupiah.trim());

  if (!isOpen) return null;

  const resetAction = () => {
    setForm(initialForm());
    setStep('form');
    setPending(false);
    setErrorBody(null);
    setLocalErrors({});
    clearExpenseAttempt(attemptRef.current);
  };

  const handleClose = () => {
    // A retained key means the last request has no terminal response yet.
    // Keep the action and payload across close/reopen so a retry cannot duplicate it.
    if (!attemptRef.current.key) resetAction();
    onClose();
  };

  const update = <K extends keyof FormState>(key: K, value: FormState[K]) => {
    setForm((current) => ({ ...current, [key]: value }));
    setLocalErrors((current) => {
      const next = { ...current };
      delete next[key];
      return next;
    });
    setErrorBody(null);
  };

  const goToConfirmation = () => {
    const errors = validateCreateExpenseForm(form);
    setLocalErrors(errors);
    setErrorBody(null);
    if (Object.keys(errors).length === 0) setStep('confirm');
  };

  const submit = async () => {
    if (pending) return;
    const errors = validateCreateExpenseForm(form);
    if (Object.keys(errors).length > 0) {
      setLocalErrors(errors);
      setStep('form');
      return;
    }
    const idempotencyKey = getExpenseAttemptKey(attemptRef.current, requestPayload);
    setPending(true);
    setErrorBody(null);
    try {
      await adminApi.createPlatformExpense(request, idempotencyKey);
      if (!mountedRef.current) return;
      resetAction();
      onCreated();
    } catch (error) {
      if (!mountedRef.current) return;
      const body = error instanceof AdminApiError
        ? error.body
        : { message: isRetryableExpenseSubmissionError(error) ? 'The request timed out or the connection was interrupted. Retry with the same request.' : 'Expense could not be created.' };
      setErrorBody(body);
      if (!isRetryableExpenseSubmissionError(error)) clearExpenseAttempt(attemptRef.current);
    } finally {
      if (mountedRef.current) setPending(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/50 p-4" role="dialog" aria-modal="true" aria-labelledby="create-expense-title">
      <div className="max-h-[90vh] w-full max-w-2xl overflow-y-auto rounded-2xl bg-white shadow-2xl">
        <div className="flex items-start justify-between border-b border-slate-200 px-6 py-5">
          <div>
            <h2 id="create-expense-title" className="text-xl font-bold text-slate-900">Add platform expense</h2>
            <p className="mt-1 text-sm text-slate-500">Record a LapangGo operating expense. This creates a DRAFT only; it does not affect P&amp;L or post a journal.</p>
          </div>
          <button type="button" onClick={handleClose} disabled={pending} className="rounded-lg p-2 text-slate-400 hover:bg-slate-100 disabled:opacity-40" aria-label="Close"><X className="h-5 w-5" /></button>
        </div>

        <div className="space-y-5 px-6 py-5">
          {step === 'form' ? (
            <>
              <div className="grid gap-4 sm:grid-cols-2">
                <label className="text-sm font-medium text-slate-700">Amount (IDR)
                  <input inputMode="numeric" value={form.amount_rupiah} onChange={(event) => update('amount_rupiah', event.target.value)} placeholder="250000" className="mt-1 block w-full rounded-lg border border-slate-200 px-3 py-2" />
                  {localErrors.amount_rupiah && <span className="mt-1 block text-xs text-rose-700">{localErrors.amount_rupiah}</span>}
                </label>
                <label className="text-sm font-medium text-slate-700">Occurred at (WIB)
                  <input type="datetime-local" value={form.occurred_at} max={formatJakartaDateTimeInput()} onChange={(event) => update('occurred_at', event.target.value)} className="mt-1 block w-full rounded-lg border border-slate-200 px-3 py-2" />
                  {localErrors.occurred_at && <span className="mt-1 block text-xs text-rose-700">{localErrors.occurred_at}</span>}
                </label>
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                <label className="text-sm font-medium text-slate-700">Category<select value={form.category} onChange={(event) => update('category', event.target.value as ExpenseCategory)} className="mt-1 block w-full rounded-lg border border-slate-200 px-3 py-2">{EXPENSE_CATEGORIES.map((item) => <option key={item} value={item}>{item}</option>)}</select></label>
                <label className="text-sm font-medium text-slate-700">Payment account<select value={form.payment_account} onChange={(event) => update('payment_account', event.target.value as ExpensePaymentAccount)} className="mt-1 block w-full rounded-lg border border-slate-200 px-3 py-2">{EXPENSE_PAYMENT_ACCOUNTS.map((item) => <option key={item} value={item}>{item}</option>)}</select></label>
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                <label className="text-sm font-medium text-slate-700">Vendor (optional)<input value={form.vendor} onChange={(event) => update('vendor', event.target.value)} maxLength={160} className="mt-1 block w-full rounded-lg border border-slate-200 px-3 py-2" />{localErrors.vendor && <span className="mt-1 block text-xs text-rose-700">{localErrors.vendor}</span>}</label>
                <label className="text-sm font-medium text-slate-700">External reference (optional)<input value={form.external_reference} onChange={(event) => update('external_reference', event.target.value)} maxLength={191} className="mt-1 block w-full rounded-lg border border-slate-200 px-3 py-2" />{localErrors.external_reference && <span className="mt-1 block text-xs text-rose-700">{localErrors.external_reference}</span>}</label>
              </div>
              <label className="text-sm font-medium text-slate-700">Description<textarea rows={3} value={form.description} onChange={(event) => update('description', event.target.value)} maxLength={500} className="mt-1 block w-full rounded-lg border border-slate-200 px-3 py-2" />{localErrors.description && <span className="mt-1 block text-xs text-rose-700">{localErrors.description}</span>}</label>
              <ErrorFields body={errorBody} localErrors={localErrors} />
              <div className="flex justify-end gap-3"><button type="button" onClick={handleClose} disabled={pending} className="rounded-lg border border-slate-200 px-4 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50">Cancel</button><button type="button" onClick={goToConfirmation} disabled={pending} className="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-semibold text-white hover:bg-emerald-700 disabled:opacity-50">Review summary</button></div>
            </>
          ) : (
            <>
              <div className="rounded-xl border border-emerald-200 bg-emerald-50 p-4"><div className="flex items-start gap-3"><CheckCircle2 className="mt-0.5 h-5 w-5 text-emerald-700" /><div><p className="font-semibold text-emerald-900">Confirm DRAFT expense</p><p className="mt-1 text-sm text-emerald-800">This records the expense for workflow review. No journal is posted and no P&amp;L summary changes until a later POSTED transition.</p></div></div></div>
              <dl className="grid gap-4 rounded-xl border border-slate-200 p-4 sm:grid-cols-2"><div><dt className="text-xs uppercase tracking-wide text-slate-400">Amount</dt><dd className="mt-1 text-lg font-bold text-slate-900">{amountLabel}</dd></div><div><dt className="text-xs uppercase tracking-wide text-slate-400">Occurred at</dt><dd className="mt-1 text-sm text-slate-700">{form.occurred_at.replace('T', ' ')} WIB</dd></div><div><dt className="text-xs uppercase tracking-wide text-slate-400">Category</dt><dd className="mt-1 text-sm text-slate-700">{form.category}</dd></div><div><dt className="text-xs uppercase tracking-wide text-slate-400">Payment account</dt><dd className="mt-1 text-sm text-slate-700">{form.payment_account}</dd></div><div className="sm:col-span-2"><dt className="text-xs uppercase tracking-wide text-slate-400">Vendor / reference</dt><dd className="mt-1 text-sm text-slate-700">{form.vendor || '—'} {form.external_reference && `· ${form.external_reference}`}</dd></div><div className="sm:col-span-2"><dt className="text-xs uppercase tracking-wide text-slate-400">Description</dt><dd className="mt-1 break-words text-sm text-slate-700">{form.description}</dd></div></dl>
              <ErrorFields body={errorBody} localErrors={localErrors} />
              {errorBody && <div className="flex items-start gap-2 rounded-lg bg-rose-50 p-3 text-sm text-rose-800"><AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />{errorBody.message}</div>}
              <div className="flex justify-between gap-3"><button type="button" onClick={() => setStep('form')} disabled={pending} className="rounded-lg border border-slate-200 px-4 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50">Back</button><button type="button" onClick={() => void submit()} disabled={pending} className="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-semibold text-white hover:bg-emerald-700 disabled:cursor-not-allowed disabled:opacity-50">{pending ? 'Saving DRAFT…' : 'Create DRAFT'}</button></div>
            </>
          )}
        </div>
      </div>
    </div>
  );
};
