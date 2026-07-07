export interface Promo {
  id: string;
  owner_id: string;
  venue_id?: string;
  code: string;
  name: string;
  description?: string;
  discount_type: 'PERCENTAGE' | 'FIXED_AMOUNT';
  discount_value: number;
  starts_at: string;
  ends_at: string;
  status: 'ACTIVE' | 'INACTIVE';
  created_at: string;
  updated_at: string;
  usage_count: number;
  total_discount_amount: number;
  total_final_revenue: number;
  can_delete: boolean;
}

export interface CreatePromoRequest {
  venue_id?: string;
  code: string;
  name: string;
  description?: string;
  discount_type: 'PERCENTAGE' | 'FIXED_AMOUNT';
  discount_value: number;
  starts_at: string;
  ends_at: string;
  status?: 'ACTIVE' | 'INACTIVE';
}
