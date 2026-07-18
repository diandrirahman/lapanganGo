export const PLATFORM_FINANCE_TIME_ZONE = 'Asia/Jakarta';
export const PLATFORM_FINANCE_OFFSET = '+07:00';

export interface ExpenseAttemptState {
  key: string | null;
  payload: string | null;
}

export const createExpenseAttemptState = (): ExpenseAttemptState => ({ key: null, payload: null });

export const getExpenseAttemptKey = (
  state: ExpenseAttemptState,
  payload: string,
  createKey: () => string = () => crypto.randomUUID(),
): string => {
  if (state.key && state.payload === payload) return state.key;
  state.key = createKey();
  state.payload = payload;
  return state.key;
};

export const clearExpenseAttempt = (state: ExpenseAttemptState): void => {
  state.key = null;
  state.payload = null;
};

export const isRetryableExpenseSubmissionError = (error: unknown): boolean => {
  if (typeof error === 'object' && error !== null && 'status' in error) {
    const status = (error as { status?: unknown }).status;
    if (typeof status === 'number') return status >= 500 || status === 408 || status === 429;
  }
  if (typeof DOMException !== 'undefined' && error instanceof DOMException && error.name === 'AbortError') return true;
  if (error instanceof Error && (error.name === 'AbortError' || error.message === 'Request timeout')) return true;
  return error instanceof TypeError;
};

const jakartaDateTimeParts = (value: Date): Record<string, string> => {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: PLATFORM_FINANCE_TIME_ZONE,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
  }).formatToParts(value);

  return parts.reduce<Record<string, string>>((result, part) => {
    if (part.type !== 'literal') result[part.type] = part.value;
    return result;
  }, {});
};

export const formatJakartaDateTimeInput = (value: Date = new Date()): string => {
  const parts = jakartaDateTimeParts(value);
  return `${parts.year}-${parts.month}-${parts.day}T${parts.hour}:${parts.minute}`;
};

export const toJakartaExpenseTimestamp = (value: string): string => {
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(value)) {
    throw new Error('Invalid Jakarta date-time input');
  }
  return `${value}:00${PLATFORM_FINANCE_OFFSET}`;
};

export const parseJakartaExpenseTimestamp = (value: string): Date | null => {
  try {
    const parsed = new Date(toJakartaExpenseTimestamp(value));
    return Number.isNaN(parsed.getTime()) ? null : parsed;
  } catch {
    return null;
  }
};

export interface ExpenseFormValidationInput {
  amount_rupiah: string;
  occurred_at: string;
  vendor: string;
  external_reference: string;
  description: string;
}

export const validateCreateExpenseForm = (form: ExpenseFormValidationInput): Record<string, string> => {
  const errors: Record<string, string> = {};
  const amount = form.amount_rupiah.trim();
  if (!/^[0-9]+$/.test(amount) || (() => {
    try {
      const value = BigInt(amount);
      return value < 1n || value > 1_000_000_000n;
    } catch {
      return true;
    }
  })()) {
    errors.amount_rupiah = 'Use a whole rupiah amount from 1 to 1,000,000,000.';
  }
  if (form.external_reference.trim() && !form.vendor.trim()) {
    errors.external_reference = 'Vendor is required when a reference is provided.';
  }
  if (new TextEncoder().encode(form.vendor.trim()).length > 160) {
    errors.vendor = 'Vendor is too long.';
  }
  if (new TextEncoder().encode(form.external_reference.trim()).length > 191) {
    errors.external_reference = 'Reference is too long.';
  }
  if (!form.description.trim() || new TextEncoder().encode(form.description.trim()).length > 500) {
    errors.description = 'Description is required and must be at most 500 bytes.';
  }

  const occurredAt = parseJakartaExpenseTimestamp(form.occurred_at);
  if (!occurredAt) {
    errors.occurred_at = 'Use a valid Jakarta date and time.';
  } else if (occurredAt.getTime() > Date.now()) {
    errors.occurred_at = 'Occurred time cannot be in the future.';
  } else if (occurredAt.getTime() < Date.now() - (90 * 24 * 60 * 60 * 1000)) {
    errors.occurred_at = 'Occurred time cannot be more than 90 days in the past.';
  }
  return errors;
};

export const validateExpenseCancelReason = (reason: string): string | null => {
  const normalized = reason.trim();
  if (!normalized) return 'A cancellation reason is required.';
  if (new TextEncoder().encode(normalized).length > 500) return 'Reason must be at most 500 bytes.';
  return null;
};

export const validateExpenseVoidReason = (reason: string): string | null => {
  const normalized = reason.trim();
  if (!normalized) return 'A void reason is required.';
  if (new TextEncoder().encode(normalized).length > 500) return 'Reason must be at most 500 bytes.';
  return null;
};
