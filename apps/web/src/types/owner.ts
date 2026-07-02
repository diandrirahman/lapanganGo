import type { Venue } from './venue';
import type { Booking } from './booking';

export interface OwnerProfile {
  id: string;
  user_id: string;
  business_name: string;
  phone_number: string;
  bank_name: string;
  bank_account_number: string;
  bank_account_name: string;
  created_at: string;
  updated_at: string;
}

export interface OwnerVenueResponse {
  venues: Venue[];
}

export interface OwnerBookingsResponse {
  bookings: Booking[];
  total: number;
  page: number;
  limit: number;
}

export interface OwnerMetrics {
  total_venues: number;
  upcoming_bookings: number;
  pending_verifications: number;
  revenue_current: number;
  booking_revenue_current?: number;
  refund_current?: number;
  net_revenue_current?: number;
  revenue_all_time: number;
  occupancy_rate: number;
}
