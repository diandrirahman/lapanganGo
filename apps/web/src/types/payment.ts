export type PaymentAttemptState =
  | 'CREATED'
  | 'PENDING'
  | 'CAPTURED'
  | 'FAILED'
  | 'EXPIRED'
  | 'CANCELLED';

export interface PaymentAttemptView {
  id: string;
  booking_id: string;
  state: PaymentAttemptState;
  expires_at: string;
  checkout_url?: string;
}

export interface ResolvePaymentAttemptResponse {
  payment_attempt: PaymentAttemptView;
  status_url: string;
}
