export interface BookingTrendItem {
  date: string;
  booking_count: number;
}

export interface BookingsTrendResponse {
  trend: BookingTrendItem[];
}

export interface RevenueTrendItem {
  date: string;
  revenue: number;
}

export interface RevenueVenueItem {
  venue_id: string;
  venue_name: string;
  revenue: number;
}

export interface RevenueResponse {
  trend: RevenueTrendItem[];
  venue_breakdown: RevenueVenueItem[];
}

export interface StatusBreakdownItem {
  status: string;
  booking_count: number;
  amount: number;
}

export interface StatusResponse {
  breakdown: StatusBreakdownItem[];
}

export interface ExpenseCategoryItem {
  category: string;
  amount: number;
}

export interface ExpensesResponse {
  breakdown: ExpenseCategoryItem[];
}
