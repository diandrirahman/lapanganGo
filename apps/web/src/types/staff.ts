export interface StaffMember {
  id: string;
  owner_profile_id: string;
  user_id: string;
  name: string;
  email: string;
  phone?: string;
  role: string;
  permissions: string[];
  status: string;
  invitation_status: string;
  invited_at?: string;
  activated_at?: string;
  invite_url?: string;
  venue_ids: string[];
  created_at: string;
  updated_at: string;
}

export interface CreateStaffRequest {
  name: string;
  email: string;
  phone?: string;
  role: string;
  permissions: string[];
  venue_ids?: string[];
}

export interface UpdateStaffRequest {
  name: string;
  phone?: string;
  role: string;
  permissions: string[];
  venue_ids?: string[];
}

export const STAFF_PERMISSIONS = [
  { id: 'BOOKINGS_READ', label: 'Melihat Pesanan', category: 'Booking' },
  { id: 'BOOKINGS_WRITE', label: 'Mengelola Pesanan', category: 'Booking' },
  { id: 'PAYMENT_VERIFY', label: 'Verifikasi Pembayaran', category: 'Booking' },
  { id: 'OFFLINE_BOOKINGS_CREATE', label: 'Membuat Pesanan Offline', category: 'Booking' },
  
  { id: 'VENUES_READ', label: 'Melihat Venue', category: 'Venue' },
  { id: 'VENUES_WRITE', label: 'Mengelola Venue', category: 'Venue' },
  { id: 'COURTS_READ', label: 'Melihat Lapangan', category: 'Venue' },
  { id: 'COURTS_WRITE', label: 'Mengelola Lapangan', category: 'Venue' },
  { id: 'SCHEDULE_READ', label: 'Melihat Jadwal', category: 'Venue' },
  { id: 'SCHEDULE_WRITE', label: 'Mengatur Jadwal', category: 'Venue' },
  { id: 'BLOCKED_SLOTS_READ', label: 'Melihat Slot Diblokir', category: 'Venue' },
  { id: 'BLOCKED_SLOTS_WRITE', label: 'Memblokir Slot', category: 'Venue' },
  
  { id: 'FINANCE_READ', label: 'Melihat Laporan Keuangan', category: 'Keuangan' },
  { id: 'FINANCE_WRITE', label: 'Mengelola Transaksi Manual', category: 'Keuangan' },
  { id: 'REFUNDS_READ', label: 'Melihat Permintaan Refund', category: 'Keuangan' },
  { id: 'REFUNDS_WRITE', label: 'Memproses Refund', category: 'Keuangan' },
  
  { id: 'PROMOS_READ', label: 'Melihat Promo', category: 'Marketing' },
  { id: 'PROMOS_WRITE', label: 'Mengelola Promo', category: 'Marketing' },
  { id: 'ANALYTICS_READ', label: 'Melihat Analitik', category: 'Laporan' },
];

export interface RegenerateInviteResponse {
  invite_url: string;
  expires_at: string;
}

export interface ResetStaffPasswordResponse {
  reset_url: string;
  expires_at: string;
}
