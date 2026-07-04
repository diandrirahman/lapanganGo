export interface Notification {
  id: string;
  type: string;
  title: string;
  message: string;
  entity_type?: 'BOOKING' | 'REFUND';
  entity_id?: string;
  read_at?: string | null;
  created_at: string;
}

export interface NotificationListResponse {
  data: Notification[] | null;
  page: number;
  limit: number;
  total: number;
  total_pages: number;
}
