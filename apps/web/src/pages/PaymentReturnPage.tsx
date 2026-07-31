import { useCallback, useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { CheckCircle2, Clock3, XCircle } from 'lucide-react';
import { ErrorState } from '../components/feedback/ErrorState';
import { LoadingState } from '../components/feedback/LoadingState';
import { PageShell } from '../components/layout/PageShell';
import { useAuth } from '../contexts/AuthContext';
import { fetchPaymentAttemptByReference } from '../lib/api';
import type { PaymentAttemptView } from '../types/payment';

const terminalStates = new Set<PaymentAttemptView['state']>([
  'CAPTURED',
  'FAILED',
  'EXPIRED',
  'CANCELLED',
]);

const initialPollDelayMs = 2_000;
const maximumPollDelayMs = 15_000;
const maximumPollWindowMs = 5 * 60_000;

const stateCopy: Record<PaymentAttemptView['state'], { title: string; message: string }> = {
  CREATED: {
    title: 'Pembayaran sedang disiapkan',
    message: 'LapangGo masih menunggu status pembayaran dari sistem pembayaran Test Mode.',
  },
  PENDING: {
    title: 'Pembayaran sedang diverifikasi',
    message: 'Jangan menutup halaman ini. Status akan diperbarui dari server secara otomatis.',
  },
  CAPTURED: {
    title: 'Pembayaran Test Mode terverifikasi',
    message: 'Server telah menerima bukti pembayaran sandbox yang terverifikasi.',
  },
  FAILED: {
    title: 'Simulasi pembayaran gagal',
    message: 'Provider Test Mode melaporkan bahwa simulasi pembayaran tidak berhasil.',
  },
  EXPIRED: {
    title: 'Sesi pembayaran kedaluwarsa',
    message: 'Sesi Test Mode ini sudah melewati batas waktu pembayaran.',
  },
  CANCELLED: {
    title: 'Pembayaran dibatalkan',
    message: 'Percobaan pembayaran Test Mode ini telah dibatalkan.',
  },
};

export function PaymentReturnPage() {
  const { reference = '', outcome = '' } = useParams();
  const { token } = useAuth();
  const [attempt, setAttempt] = useState<PaymentAttemptView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);
  const validOutcome = outcome === 'success' || outcome === 'cancel';

  const reload = useCallback(() => {
    setError(null);
    setReloadKey((value) => value + 1);
  }, []);

  useEffect(() => {
    if (!token || !validOutcome) {
      return;
    }

    const controller = new AbortController();
    let pollTimer: ReturnType<typeof setTimeout> | undefined;
    let expiryTimer: ReturnType<typeof setTimeout> | undefined;
    let deadlineTimer: ReturnType<typeof setTimeout> | undefined;
    const pollingDeadline = Date.now() + maximumPollWindowMs;
    let knownAttemptExpiry: number | undefined;
    let pollNumber = 0;
    const deadlineMessage = 'Status pembayaran masih belum final setelah batas waktu pemeriksaan. Coba lagi dari detail booking.';
    const expiryMessage = 'Sesi pembayaran sudah melewati waktu kedaluwarsa. Status final akan dipastikan oleh server.';

    const clearTimers = () => {
      if (pollTimer) {
        clearTimeout(pollTimer);
      }
      if (expiryTimer) {
        clearTimeout(expiryTimer);
      }
      if (deadlineTimer) {
        clearTimeout(deadlineTimer);
      }
    };

    const stopWithError = (message: string) => {
      if (controller.signal.aborted) {
        return;
      }
      clearTimers();
      controller.abort();
      setError(message);
    };

    deadlineTimer = setTimeout(
      () => stopWithError(deadlineMessage),
      maximumPollWindowMs,
    );

    const poll = async () => {
      const now = Date.now();
      if (now >= pollingDeadline) {
        stopWithError(deadlineMessage);
        return;
      }
      if (knownAttemptExpiry !== undefined && now >= knownAttemptExpiry) {
        stopWithError(expiryMessage);
        return;
      }

      try {
        const result = await fetchPaymentAttemptByReference(reference, token, controller.signal);
        setAttempt(result.payment_attempt);
        setError(null);
        if (terminalStates.has(result.payment_attempt.state)) {
          clearTimers();
          return;
        }

        knownAttemptExpiry = Date.parse(result.payment_attempt.expires_at);
        if (!Number.isFinite(knownAttemptExpiry)) {
          setError('Batas waktu pembayaran dari server tidak valid. Buka detail booking untuk memeriksa status.');
          return;
        }

        const remainingUntilExpiry = knownAttemptExpiry - Date.now();
        const remainingPollWindow = pollingDeadline - Date.now();
        if (remainingUntilExpiry <= 0) {
          stopWithError(expiryMessage);
          return;
        }
        if (remainingPollWindow <= 0) {
          stopWithError(deadlineMessage);
          return;
        }

        if (expiryTimer) {
          clearTimeout(expiryTimer);
        }
        expiryTimer = setTimeout(
          () => stopWithError(expiryMessage),
          remainingUntilExpiry,
        );

        const backoffDelay = Math.min(
          initialPollDelayMs * (2 ** Math.min(pollNumber, 3)),
          maximumPollDelayMs,
        );
        pollNumber += 1;
        pollTimer = setTimeout(
          poll,
          Math.max(1, Math.min(backoffDelay, remainingUntilExpiry, remainingPollWindow)),
        );
      } catch (requestError) {
        if (!controller.signal.aborted) {
          stopWithError(
            requestError instanceof Error
              ? requestError.message
              : 'Status pembayaran tidak dapat dimuat',
          );
        }
      }
    };

    void poll();
    return () => {
      clearTimers();
      controller.abort();
    };
  }, [reference, reloadKey, token, validOutcome]);

  const browserOutcome = outcome === 'cancel'
    ? 'Anda kembali dari halaman pembatalan checkout.'
    : 'Anda kembali dari halaman checkout.';

  return (
    <PageShell>
      <main className="mx-auto min-h-[70vh] max-w-3xl px-4 pb-20 pt-28 sm:px-6">
        <div className="rounded-3xl border border-slate-200 bg-white p-6 shadow-sm sm:p-10">
          <div className="mb-8 rounded-2xl border border-orange-200 bg-orange-50 px-4 py-3 text-sm font-semibold text-orange-800">
            Test Mode — tidak ada uang asli, payout, atau settlement.
          </div>

          {!validOutcome && (
            <ErrorState
              title="Alamat return pembayaran tidak valid"
              message="Status checkout tidak dikenali. Buka kembali detail booking untuk melihat status pembayaran dari server."
            />
          )}
          {validOutcome && !attempt && !error && <LoadingState message="Memeriksa status pembayaran dari server..." />}
          {validOutcome && error && (
            <ErrorState
              title="Status pembayaran belum tersedia"
              message={error}
              onRetry={reload}
            />
          )}
          {validOutcome && attempt && !error && (
            <div className="text-center">
              {attempt.state === 'CAPTURED' ? (
                <CheckCircle2 className="mx-auto mb-5 h-14 w-14 text-emerald-500" />
              ) : terminalStates.has(attempt.state) ? (
                <XCircle className="mx-auto mb-5 h-14 w-14 text-red-500" />
              ) : (
                <Clock3 className="mx-auto mb-5 h-14 w-14 text-amber-500" />
              )}
              <p className="mb-2 text-sm font-semibold text-slate-500">{browserOutcome}</p>
              <h1 className="text-2xl font-extrabold text-slate-900 sm:text-3xl">
                {stateCopy[attempt.state].title}
              </h1>
              <p className="mx-auto mt-3 max-w-xl text-slate-600">
                {stateCopy[attempt.state].message}
              </p>
              <p className="mt-4 text-xs text-slate-500">
                Tampilan ini hanya mengikuti status lokal yang diverifikasi server; alamat return tidak pernah menjadi bukti pembayaran.
              </p>
              <div className="mt-8 flex flex-col justify-center gap-3 sm:flex-row">
                <Link
                  to={`/bookings/${attempt.booking_id}`}
                  className="rounded-xl bg-primary px-5 py-3 text-sm font-bold text-white transition hover:opacity-90"
                >
                  Lihat detail booking
                </Link>
                <Link
                  to="/bookings"
                  className="rounded-xl border border-slate-200 px-5 py-3 text-sm font-bold text-slate-700 transition hover:bg-slate-50"
                >
                  Daftar booking
                </Link>
              </div>
            </div>
          )}
        </div>
      </main>
    </PageShell>
  );
}
