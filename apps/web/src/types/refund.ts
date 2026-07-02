export interface RefundRequest {
  id: string;
  booking_id: string;
  customer_id: string;
  owner_id: string;
  venue_id?: string;
  reason: string;
  status: 'PENDING' | 'APPROVED' | 'REJECTED' | 'CANCELLED';
  owner_note?: string;
  requested_at: string;
  reviewed_at?: string;
  reviewed_by_user_id?: string;
  created_at: string;
  updated_at: string;
}

export interface OwnerRefundRequest {
  id: string;
  booking_id: string;
  customer_name: string;
  customer_email: string;
  venue_name: string;
  court_name: string;
  booking_date: string;
  start_time: string;
  end_time: string;
  amount: number;
  reason: string;
  status: 'PENDING' | 'APPROVED' | 'REJECTED' | 'CANCELLED';
  requested_at: string;
}

export interface PaginatedOwnerRefundRequests {
  data: OwnerRefundRequest[];
  total: number;
  page: number;
  limit: number;
  total_pages: number;
}
