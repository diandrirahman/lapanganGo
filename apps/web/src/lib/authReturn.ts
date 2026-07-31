const PAYMENT_RETURN_STORAGE_KEY = 'lapanggo:payment-return-to';
const paymentReturnPathPattern = /^\/payments\/return\/pa-[0-9a-f]{60}\/(?:success|cancel)$/;

export function isSafePaymentReturnPath(pathname: string): boolean {
  return paymentReturnPathPattern.test(pathname);
}

export function rememberPaymentReturnPath(pathname: string): void {
  if (typeof window === 'undefined') {
    return;
  }
  try {
    if (!isSafePaymentReturnPath(pathname)) {
      window.sessionStorage.removeItem(PAYMENT_RETURN_STORAGE_KEY);
      return;
    }
    window.sessionStorage.setItem(PAYMENT_RETURN_STORAGE_KEY, pathname);
  } catch {
    // Authentication must still fail closed when browser storage is disabled.
  }
}

export function consumePaymentReturnPath(role?: string): string | null {
  if (typeof window === 'undefined') {
    return null;
  }
  try {
    const pathname = window.sessionStorage.getItem(PAYMENT_RETURN_STORAGE_KEY);
    window.sessionStorage.removeItem(PAYMENT_RETURN_STORAGE_KEY);
    return role === 'CUSTOMER' && pathname && isSafePaymentReturnPath(pathname)
      ? pathname
      : null;
  } catch {
    return null;
  }
}
