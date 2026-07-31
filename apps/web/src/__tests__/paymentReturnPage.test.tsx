import { act, cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ProtectedRoute } from '../components/ProtectedRoute';
import {
  consumePaymentReturnPath,
  rememberPaymentReturnPath,
} from '../lib/authReturn';
import { PaymentReturnPage } from '../pages/PaymentReturnPage';

const testState = vi.hoisted(() => ({
  fetchPaymentAttemptByReference: vi.fn(),
  auth: {
    token: 'customer-test-token' as string | null,
    user: {
      id: 'customer-1',
      name: 'Customer Test',
      email: 'customer@example.test',
      role: 'CUSTOMER',
    } as { id: string; name: string; email: string; role: string } | null,
    isAuthenticated: true,
    isLoading: false,
  },
}));

const paymentReference = `pa-${'a'.repeat(60)}`;
const paymentReturnPath = `/payments/return/${paymentReference}/success`;

vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({
    ...testState.auth,
    isWorkspaceUser: () => false,
    isActualOwner: () => false,
    hasOwnerPermission: () => false,
    logout: vi.fn(),
  }),
}));

vi.mock('../lib/api', async (importOriginal) => {
  const original = await importOriginal<typeof import('../lib/api')>();
  return {
    ...original,
    fetchPaymentAttemptByReference: (...args: unknown[]) =>
      testState.fetchPaymentAttemptByReference(...args),
    fetchUnreadNotificationCount: vi.fn().mockResolvedValue({ count: 0 }),
  };
});

beforeEach(() => {
  window.sessionStorage.clear();
  testState.auth.token = 'customer-test-token';
  testState.auth.user = {
    id: 'customer-1',
    name: 'Customer Test',
    email: 'customer@example.test',
    role: 'CUSTOMER',
  };
  testState.auth.isAuthenticated = true;
  testState.auth.isLoading = false;
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  testState.fetchPaymentAttemptByReference.mockReset();
  window.sessionStorage.clear();
});

describe('PaymentReturnPage', () => {
  it('uses server state instead of treating a success return path as payment authority', async () => {
    testState.fetchPaymentAttemptByReference.mockResolvedValue({
      payment_attempt: {
        id: 'attempt-1',
        booking_id: 'booking-1',
        state: 'PENDING',
        expires_at: new Date(Date.now() + 60_000).toISOString(),
      },
      status_url: '/payment-attempts/attempt-1',
    });

    render(
      <MemoryRouter initialEntries={[paymentReturnPath]}>
        <Routes>
          <Route path="/payments/return/:reference/:outcome" element={<PaymentReturnPage />} />
        </Routes>
      </MemoryRouter>,
    );

    expect(await screen.findByText('Pembayaran sedang diverifikasi')).toBeTruthy();
    expect(screen.queryByText('Pembayaran Test Mode terverifikasi')).toBeNull();
    expect(screen.getByText(/alamat return tidak pernah menjadi bukti pembayaran/i)).toBeTruthy();
    expect(testState.fetchPaymentAttemptByReference).toHaveBeenCalledWith(
      paymentReference,
      'customer-test-token',
      expect.any(AbortSignal),
    );
  });

  it('stops polling when a pending payment attempt is already expired', async () => {
    testState.fetchPaymentAttemptByReference.mockResolvedValue({
      payment_attempt: {
        id: 'attempt-expired',
        booking_id: 'booking-1',
        state: 'PENDING',
        expires_at: new Date(Date.now() - 1_000).toISOString(),
      },
      status_url: '/payment-attempts/attempt-expired',
    });

    render(
      <MemoryRouter initialEntries={[paymentReturnPath]}>
        <Routes>
          <Route path="/payments/return/:reference/:outcome" element={<PaymentReturnPage />} />
        </Routes>
      </MemoryRouter>,
    );

    expect(await screen.findByText(/sudah melewati waktu kedaluwarsa/i)).toBeTruthy();
    expect(testState.fetchPaymentAttemptByReference).toHaveBeenCalledTimes(1);
  });

  it('stops polling after the bounded verification window', async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-07-30T00:00:00Z'));
    testState.fetchPaymentAttemptByReference.mockResolvedValue({
      payment_attempt: {
        id: 'attempt-pending',
        booking_id: 'booking-1',
        state: 'PENDING',
        expires_at: '2026-07-30T01:00:00Z',
      },
      status_url: '/payment-attempts/attempt-pending',
    });

    render(
      <MemoryRouter initialEntries={[paymentReturnPath]}>
        <Routes>
          <Route path="/payments/return/:reference/:outcome" element={<PaymentReturnPage />} />
        </Routes>
      </MemoryRouter>,
    );

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5 * 60_000 + 1);
    });

    expect(screen.getByText(/belum final setelah batas waktu pemeriksaan/i)).toBeTruthy();
    expect(testState.fetchPaymentAttemptByReference.mock.calls.length).toBeGreaterThan(1);
    expect(testState.fetchPaymentAttemptByReference.mock.calls.length).toBeLessThan(30);
  });

  it('aborts a hanging status request when the verification window ends', async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-07-30T00:00:00Z'));
    testState.fetchPaymentAttemptByReference.mockImplementation(
      () => new Promise(() => undefined),
    );

    render(
      <MemoryRouter initialEntries={[paymentReturnPath]}>
        <Routes>
          <Route path="/payments/return/:reference/:outcome" element={<PaymentReturnPage />} />
        </Routes>
      </MemoryRouter>,
    );

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5 * 60_000);
    });

    expect(screen.getByText(/belum final setelah batas waktu pemeriksaan/i)).toBeTruthy();
    const signal = testState.fetchPaymentAttemptByReference.mock.calls[0][2] as AbortSignal;
    expect(signal.aborted).toBe(true);
  });

  it('remembers an authenticated payment return route when login is required', () => {
    testState.auth.token = null;
    testState.auth.user = null;
    testState.auth.isAuthenticated = false;

    render(
      <MemoryRouter initialEntries={[paymentReturnPath]}>
        <Routes>
          <Route element={<ProtectedRoute requiredRole="CUSTOMER" />}>
            <Route
              path="/payments/return/:reference/:outcome"
              element={<div>Payment return</div>}
            />
          </Route>
          <Route path="/login" element={<div>Login required</div>} />
        </Routes>
      </MemoryRouter>,
    );

    expect(screen.getByText('Login required')).toBeTruthy();
    expect(consumePaymentReturnPath('CUSTOMER')).toBe(paymentReturnPath);
  });

  it('fails closed for an unsupported outcome without resolving payment state', () => {
    render(
      <MemoryRouter initialEntries={[`/payments/return/${paymentReference}/failed`]}>
        <Routes>
          <Route path="/payments/return/:reference/:outcome" element={<PaymentReturnPage />} />
        </Routes>
      </MemoryRouter>,
    );

    expect(screen.getByText('Alamat return pembayaran tidak valid')).toBeTruthy();
    expect(testState.fetchPaymentAttemptByReference).not.toHaveBeenCalled();
  });

  it('only restores a canonical payment return route for a customer', () => {
    for (const unsafePath of [
      'https://evil.example/payments/return',
      '//evil.example/payments/return',
      `/payments/return/${paymentReference}/failed`,
      '/payments/return/pa-short/success',
      '/owner/dashboard',
    ]) {
      rememberPaymentReturnPath(paymentReturnPath);
      rememberPaymentReturnPath(unsafePath);
      expect(consumePaymentReturnPath('CUSTOMER')).toBeNull();
    }

    rememberPaymentReturnPath(paymentReturnPath);
    expect(consumePaymentReturnPath('OWNER')).toBeNull();
    expect(consumePaymentReturnPath('CUSTOMER')).toBeNull();

    rememberPaymentReturnPath(paymentReturnPath);
    expect(consumePaymentReturnPath('CUSTOMER')).toBe(paymentReturnPath);
    expect(consumePaymentReturnPath('CUSTOMER')).toBeNull();
  });
});
