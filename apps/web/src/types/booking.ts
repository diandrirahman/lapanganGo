export interface AvailabilitySlot {
  start_at: string;
  end_at: string;
  status: 'AVAILABLE' | 'BOOKED' | 'BLOCKED';
}

export interface AvailabilityResponse {
  court_id: string;
  date: string;
  status: 'OPEN' | 'CLOSED';
  slots: AvailabilitySlot[];
}

export interface VenueSummary {
  id: string;
  name: string;
  address?: string;
  city?: string;
}

export interface CourtSummary {
  id: string;
  name: string;
  sport_name?: string;
}

export interface CreateBookingRequest {
  court_id: string;
  booking_date: string;
  start_time: string;
  end_time: string;
}

export interface Booking {
  id: string;
  customer_id: string;
  venue?: VenueSummary;
  court?: CourtSummary;
  court_id: string;
  booking_date: string;
  start_time: string;
  end_time: string;
  total_price: number;
  status: 'PENDING_PAYMENT' | 'PAID' | 'CONFIRMED' | 'CANCELLED' | 'WAITING_VERIFICATION' | 'COMPLETED';
  payment_reference?: string;
  expires_at?: string;
  created_at: string;
  updated_at: string;
}

export interface OwnerBooking {
  id: string;
  customer: {
    id: string;
    name: string;
    email: string;
    phone?: string;
  };
  venue: VenueSummary;
  court: CourtSummary;
  booking_date: string;
  start_time: string;
  end_time: string;
  total_price: number;
  status: 'PENDING_PAYMENT' | 'PAID' | 'CONFIRMED' | 'CANCELLED' | 'WAITING_VERIFICATION' | 'COMPLETED';
  payment_reference?: string;
  expires_at?: string;
  created_at: string;
  updated_at: string;
}

export interface OwnerCreateOfflineBookingRequest {
  venue_id: string;
  court_id: string;
  booking_date: string;
  start_time: string;
  end_time: string;
  customer_name: string;
  customer_phone?: string;
  customer_email?: string;
  total_price: number;
  status: 'PAID' | 'COMPLETED';
  note?: string;
}
