import type { Venue, VenueDetail, Court, OperatingHour, BlockedSlot, Sport, Facility } from '../types/venue';
import type { AvailabilityResponse, CreateBookingRequest, Booking, OwnerBooking } from '../types/booking';
import type { OwnerProfile, OwnerMetrics } from '../types/owner';
import type { PaginatedResponse } from '../types/pagination';
import type { OpenMatch } from '../types/mabar';
import type { FinanceSummaryResult } from '../types/finance';

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080';

async function apiFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  const response = await fetch(input, init);
  if (response.status === 401) {
    if (typeof window !== 'undefined' && window.location.pathname !== '/login') {
      localStorage.removeItem('auth_token');
      window.location.href = '/login';
    }
  }
  return response;
}

export async function fetchOpenMatches(page: number = 1, limit: number = 10): Promise<PaginatedResponse<OpenMatch>> {
  if (import.meta.env.VITE_USE_MOCK_MABAR === 'true') {
    return new Promise((resolve) => {
      setTimeout(() => resolve({
        data: [{
          id: 'mock-1',
          booking_id: 'booking-1',
          title: 'Mabar Santuy - Fun Game',
          description: 'Mabar santai sore hari, level pemula/menengah. Yang penting keringat!',
          status: 'OPEN',
          joined_count: 3,
          level: 'Beginner',
          max_players: 10,
          remaining_slots: 7,
          price_per_player: 35000,
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
          venue_name: 'Gor Soemantri',
          court_name: 'Lapangan Badminton 1',
          sport_name: 'Badminton',
          match_date: new Date().toISOString().split('T')[0],
          start_time: '19:00',
          end_time: '21:00',
          host_user_id: 'user-1',
          host_name: 'Budi Santoso'
        }],
        page: 1,
        limit: 10,
        total: 1,
        total_pages: 1
      }), 500);
    });
  }

  const response = await apiFetch(`${API_BASE_URL}/open-matches?page=${page}&limit=${limit}`);
  if (!response.ok) {
    throw new Error('Gagal mengambil data mabar');
  }
  return response.json();
}

import type { OpenMatchDetailResponse } from '../types/mabar';

export async function fetchOpenMatchById(id: string): Promise<OpenMatchDetailResponse> {
  if (import.meta.env.VITE_USE_MOCK_MABAR === 'true') {
    return new Promise((resolve) => {
      setTimeout(() => resolve({
        open_match: {
          id: id,
          booking_id: 'mock-booking-1',
          host_user_id: 'mock-user-1',
          host_name: 'Budi Santoso',
          title: 'Fun Match Basket Akhir Pekan',
          description: 'Main santai aja, cari keringat. Patungan ya.',
          sport_name: 'Basketball',
          venue_name: 'Gor Soemantri',
          court_name: 'Lapangan Basket 1',
          match_date: new Date().toISOString().split('T')[0],
          start_time: '14:00',
          end_time: '16:00',
          level: 'BEGINNER',
          max_players: 10,
          joined_count: 4,
          remaining_slots: 6,
          price_per_player: 30000,
          status: 'OPEN',
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString()
        },
        participants: [
          { id: 'p1', user_id: 'u1', name: 'Budi Santoso', status: 'JOINED', joined_at: new Date().toISOString() },
          { id: 'p2', user_id: 'u2', name: 'Andi', status: 'JOINED', joined_at: new Date().toISOString() }
        ]
      }), 800);
    });
  }

  const response = await apiFetch(`${API_BASE_URL}/open-matches/${id}`);
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Failed to fetch open match details');
  }
  return response.json();
}

export async function joinOpenMatch(id: string, token: string): Promise<void> {
  if (import.meta.env.VITE_USE_MOCK_MABAR === 'true') {
    return new Promise((resolve) => setTimeout(resolve, 800));
  }
  const response = await apiFetch(`${API_BASE_URL}/open-matches/${id}/join`, {
    method: 'POST',
    headers: { 'Authorization': `Bearer ${token}` }
  });
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal bergabung ke mabar');
  }
}

export async function leaveOpenMatch(id: string, token: string): Promise<void> {
  if (import.meta.env.VITE_USE_MOCK_MABAR === 'true') {
    return new Promise((resolve) => setTimeout(resolve, 800));
  }
  const response = await apiFetch(`${API_BASE_URL}/open-matches/${id}/join`, {
    method: 'DELETE',
    headers: { 'Authorization': `Bearer ${token}` }
  });
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal keluar dari mabar');
  }
}

export async function createOpenMatch(bookingId: string, data: any, token: string): Promise<OpenMatch> {
  if (import.meta.env.VITE_USE_MOCK_MABAR === 'true') {
    return new Promise((resolve) => setTimeout(() => resolve(data as OpenMatch), 800));
  }
  const response = await apiFetch(`${API_BASE_URL}/bookings/${bookingId}/open-matches`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`
    },
    body: JSON.stringify(data)
  });
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal membuat mabar');
  }
  const result = await response.json();
  return result.open_match;
}

export async function cancelOpenMatch(id: string, token: string): Promise<void> {
  if (import.meta.env.VITE_USE_MOCK_MABAR === 'true') {
    return new Promise((resolve) => setTimeout(resolve, 800));
  }
  const response = await apiFetch(`${API_BASE_URL}/open-matches/${id}/cancel`, {
    method: 'PATCH',
    headers: { 'Authorization': `Bearer ${token}` }
  });
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal membatalkan mabar');
  }
}

const MOCK_VENUES: Venue[] = [
  {
    id: "v1-gbk-alpha",
    name: "GBK Alpha Field",
    address: "Senayan",
    city: "Jakarta Pusat",
    facilities: [
      { id: "f1", name: "Toilet" },
      { id: "f2", name: "Kantin" }
    ],
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  },
  {
    id: "v2-elite-tennis",
    name: "Elite Tennis Club",
    address: "Kemang",
    city: "Jakarta Selatan",
    facilities: [
      { id: "f1", name: "Toilet" },
      { id: "f3", name: "Locker Room" }
    ],
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  },
  {
    id: "v3-strike-billiard",
    name: "Strike & Pocket Lounge",
    address: "Menteng",
    city: "Jakarta Pusat",
    facilities: [
      { id: "f1", name: "Toilet" },
      { id: "f4", name: "AC" }
    ],
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  }
];

export async function fetchVenues(page: number = 1, limit: number = 10, filters?: { q?: string; city?: string; sport_id?: string; facility_ids?: string[]; min_price?: number; max_price?: number }): Promise<PaginatedResponse<Venue>> {
  if (import.meta.env.VITE_USE_MOCK_VENUE === 'true') {
    return new Promise((resolve) => setTimeout(() => resolve({ data: MOCK_VENUES, page: 1, limit: 10, total: MOCK_VENUES.length, total_pages: 1 }), 500));
  }
  
  const params = new URLSearchParams({
    page: page.toString(),
    limit: limit.toString()
  });

  if (filters?.q) params.append('q', filters.q);
  if (filters?.city) params.append('city', filters.city);
  if (filters?.sport_id) params.append('sport_id', filters.sport_id);
  if (filters?.min_price) params.append('min_price', filters.min_price.toString());
  if (filters?.max_price) params.append('max_price', filters.max_price.toString());
  if (filters?.facility_ids && filters.facility_ids.length > 0) {
    filters.facility_ids.forEach(id => params.append('facility_ids', id));
  }

  const response = await apiFetch(`${API_BASE_URL}/venues?${params.toString()}`);
  if (!response.ok) {
    throw new Error(`Failed to fetch venues: ${response.status} ${response.statusText}`);
  }
  const data = await response.json();
  return data;
}

export async function fetchVenueById(id: string): Promise<VenueDetail> {
  if (import.meta.env.VITE_USE_MOCK_VENUE === 'true') {
    return new Promise((resolve, reject) => {
      setTimeout(() => {
        const venue = MOCK_VENUES.find(v => v.id === id);
        if (venue) {
          resolve({
            ...venue,
            courts: [
              {
                id: 'c1',
                sport: { id: 's1', name: 'Mini Soccer' },
                name: 'Lapangan Utama',
                location_type: 'OUTDOOR',
                surface_type: 'SYNTHETIC_GRASS',
                price_per_hour: 450000,
                created_at: new Date().toISOString(),
                updated_at: new Date().toISOString()
              },
              {
                id: 'c2',
                sport: { id: 's1', name: 'Mini Soccer' },
                name: 'Lapangan Indoor',
                location_type: 'INDOOR',
                surface_type: 'SYNTHETIC_GRASS',
                price_per_hour: 500000,
                created_at: new Date().toISOString(),
                updated_at: new Date().toISOString()
              }
            ]
          });
        } else {
          reject(new Error('Venue not found'));
        }
      }, 500);
    });
  }

  const response = await apiFetch(`${API_BASE_URL}/venues/${id}`);
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Failed to fetch venue details');
  }

  return response.json();
}
export async function fetchCourtAvailability(courtId: string, date: string): Promise<AvailabilityResponse> {
  if (import.meta.env.VITE_USE_MOCK_VENUE === 'true') {
    return new Promise((resolve) => {
      setTimeout(() => {
        const slots: AvailabilityResponse['slots'] = [];
        for (let i = 6; i <= 22; i++) {
          const startAt = new Date(`${date}T${i.toString().padStart(2, '0')}:00:00+07:00`).toISOString();
          const endAt = new Date(`${date}T${(i + 1).toString().padStart(2, '0')}:00:00+07:00`).toISOString();
          let status: 'AVAILABLE' | 'BOOKED' | 'BLOCKED' = 'AVAILABLE';
          
          if (i === 8 || i === 18 || i === 19) status = 'BOOKED';
          else if (i === 12 || i === 13) status = 'BLOCKED';

          slots.push({
            start_at: startAt,
            end_at: endAt,
            status
          });
        }
        resolve({
          court_id: courtId,
          date,
          status: 'OPEN',
          slots
        });
      }, 500);
    });
  }

  const response = await apiFetch(`${API_BASE_URL}/courts/${courtId}/availability?date=${date}`);
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Failed to fetch availability');
  }

  return response.json();
}

export const createBooking = async (data: CreateBookingRequest, token: string): Promise<Booking> => {
  const isMock = import.meta.env.VITE_USE_MOCK_VENUE === 'true';
  
  if (isMock) {
    return new Promise((resolve) => {
      setTimeout(() => {
        resolve({
          id: 'booking-mock-id',
          customer_id: 'mock-cust-id',
          court_id: data.court_id,
          booking_date: data.booking_date,
          start_time: data.start_time,
          end_time: data.end_time,
          total_price: 150000,
          status: 'PENDING_PAYMENT',
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
        });
      }, 1000);
    });
  }

  const response = await apiFetch(`${API_BASE_URL}/bookings`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`
    },
    body: JSON.stringify(data)
  });

  if (!response.ok) {
    const errorData = await response.json();
    throw new Error(errorData.message || 'Gagal membuat pesanan');
  }

  const result = await response.json();
  return result.booking;
};

export async function fetchOwnerVenueById(id: string, token: string): Promise<Venue> {
  const response = await apiFetch(`${API_BASE_URL}/owner/venues/${id}`, {
    headers: { 'Authorization': `Bearer ${token}` }
  });
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal mengambil detail venue owner');
  }
  const data = await response.json();
  return data.venue;
}

export async function fetchOwnerCourtsByVenueId(venueId: string, token: string): Promise<Court[]> {
  const response = await apiFetch(`${API_BASE_URL}/owner/venues/${venueId}/courts`, {
    headers: { 'Authorization': `Bearer ${token}` }
  });
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal mengambil daftar lapangan');
  }
  const data = await response.json();
  return data.courts || [];
}

export async function createOwnerVenue(data: any, token: string): Promise<Venue> {
  const response = await apiFetch(`${API_BASE_URL}/owner/venues`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`
    },
    body: JSON.stringify(data)
  });
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal mendaftar venue');
  }
  const result = await response.json();
  return result.venue || result;
}

export async function updateOwnerVenue(venueId: string, data: any, token: string): Promise<Venue> {
  const response = await apiFetch(`${API_BASE_URL}/owner/venues/${venueId}`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`
    },
    body: JSON.stringify(data)
  });
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal mengubah venue');
  }
  const result = await response.json();
  return result.venue || result;
}

export async function addVenuePhoto(venueId: string, data: any, token: string): Promise<any> {
  const response = await apiFetch(`${API_BASE_URL}/owner/venues/${venueId}/photos`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`
    },
    body: JSON.stringify(data)
  });
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal menambah foto venue');
  }
  return await response.json();
}

export async function updateVenuePhoto(venueId: string, photoId: string, data: any, token: string): Promise<any> {
  const response = await apiFetch(`${API_BASE_URL}/owner/venues/${venueId}/photos/${photoId}`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`
    },
    body: JSON.stringify(data)
  });
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal mengubah foto venue');
  }
  return await response.json();
}

export async function deleteVenuePhoto(venueId: string, photoId: string, token: string): Promise<any> {
  const response = await apiFetch(`${API_BASE_URL}/owner/venues/${venueId}/photos/${photoId}`, {
    method: 'DELETE',
    headers: {
      'Authorization': `Bearer ${token}`
    }
  });
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal menghapus foto venue');
  }
  return await response.json();
}

export async function createOwnerCourt(venueId: string, data: any, token: string): Promise<Court> {
  const response = await apiFetch(`${API_BASE_URL}/owner/venues/${venueId}/courts`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`
    },
    body: JSON.stringify(data)
  });
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal membuat lapangan');
  }
  const result = await response.json();
  return result.court;
}

export async function updateOwnerCourt(courtId: string, data: any, token: string): Promise<Court> {
  const response = await apiFetch(`${API_BASE_URL}/owner/courts/${courtId}`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`
    },
    body: JSON.stringify(data)
  });
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal memperbarui lapangan');
  }
  const result = await response.json();
  return result.court;
}

export async function getOperatingHours(courtId: string, token: string): Promise<OperatingHour[]> {
  const response = await apiFetch(`${API_BASE_URL}/owner/courts/${courtId}/operating-hours`, {
    headers: { 'Authorization': `Bearer ${token}` }
  });
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal mengambil jam operasional');
  }
  const data = await response.json();
  return data.operating_hours || [];
}

export async function updateOperatingHours(courtId: string, data: any, token: string): Promise<OperatingHour[]> {
  const response = await apiFetch(`${API_BASE_URL}/owner/courts/${courtId}/operating-hours`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`
    },
    body: JSON.stringify({ days: data })
  });
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal memperbarui jam operasional');
  }
  const resData = await response.json();
  return resData.operating_hours || [];
}

export async function getBlockedSlots(courtId: string, token: string): Promise<BlockedSlot[]> {
  const response = await apiFetch(`${API_BASE_URL}/owner/courts/${courtId}/blocked-slots`, {
    headers: { 'Authorization': `Bearer ${token}` }
  });
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal mengambil jadwal blokir');
  }
  const data = await response.json();
  return data.blocked_slots || [];
}

export async function createBlockedSlot(courtId: string, data: any, token: string): Promise<BlockedSlot> {
  const response = await apiFetch(`${API_BASE_URL}/owner/courts/${courtId}/blocked-slots`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`
    },
    body: JSON.stringify(data)
  });
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal membuat jadwal blokir');
  }
  const result = await response.json();
  return result.blocked_slot;
}

export async function deleteBlockedSlot(id: string, token: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/owner/blocked-slots/${id}`, {
    method: 'DELETE',
    headers: { 'Authorization': `Bearer ${token}` }
  });
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal menghapus jadwal blokir');
  }
}


export async function fetchSports(): Promise<Sport[]> {
  const response = await apiFetch(`${API_BASE_URL}/sports`);
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal mengambil data olahraga');
  }
  const data = await response.json();
  return data.sports || [];
}

export async function fetchFacilities(): Promise<Facility[]> {
  const response = await apiFetch(`${API_BASE_URL}/facilities`);
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal mengambil data fasilitas');
  }
  const data = await response.json();
  return data.facilities || [];
}

export const fetchCustomerBookings = async (token: string, page: number = 1, limit: number = 10): Promise<PaginatedResponse<Booking>> => {
  const isMock = import.meta.env.VITE_USE_MOCK_VENUE === 'true';

  if (isMock) {
    return new Promise((resolve) => {
      setTimeout(() => {
        resolve({
          data: [
            {
              id: 'mock-booking-1',
              customer_id: 'mock-cust-id',
              venue: { id: 'mock-venue-1', name: 'Gor Soemantri' },
              court: { id: 'mock-court-1', name: 'Lapangan Basket 1' },
              court_id: 'mock-court-1',
              booking_date: new Date().toISOString().split('T')[0],
              start_time: '14:00',
              end_time: '16:00',
              total_price: 300000,
              status: 'PENDING_PAYMENT',
              created_at: new Date().toISOString(),
              updated_at: new Date().toISOString(),
            },
            {
              id: 'mock-booking-2',
              customer_id: 'mock-cust-id',
              venue: { id: 'mock-venue-2', name: 'Tifosi Futsal' },
              court: { id: 'mock-court-2', name: 'Lapangan Futsal C' },
              court_id: 'mock-court-2',
              booking_date: new Date().toISOString().split('T')[0],
              start_time: '19:00',
              end_time: '21:00',
              total_price: 250000,
              status: 'CONFIRMED',
              created_at: new Date().toISOString(),
              updated_at: new Date().toISOString(),
            }
          ],
          page: 1,
          total_pages: 1,
          total: 2,
          limit: 10
        });
      }, 1000);
    });
  }

  const res = await apiFetch(`${API_BASE_URL}/bookings?page=${page}&limit=${limit}`, {
    headers: { 'Authorization': `Bearer ${token}` }
  });
  if (!res.ok) {
    const errorData = await res.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal memuat daftar pesanan');
  }
  return res.json();
};


export const cancelBooking = async (id: string, token: string): Promise<Booking> => {
  const isMock = import.meta.env.VITE_USE_MOCK_VENUE === 'true';

  if (isMock) {
    return new Promise((resolve) => {
      setTimeout(() => {
        resolve({} as Booking); // simplified for mock
      }, 500);
    });
  }

  const response = await apiFetch(`${API_BASE_URL}/bookings/${id}/cancel`, {
    method: 'PATCH',
    headers: {
      'Authorization': `Bearer ${token}`
    }
  });

  if (!response.ok) {
    const errorData = await response.json();
    throw new Error(errorData.message || 'Gagal membatalkan pesanan');
  }

  const data = await response.json();
  return data.booking;
};


export const submitPaymentProof = async (id: string, reference: string, token: string): Promise<Booking> => {
  const isMock = import.meta.env.VITE_USE_MOCK_VENUE === 'true';

  if (isMock) {
    return new Promise((resolve) => {
      setTimeout(() => {
        resolve({
          id,
          customer_id: 'mock-cust-id',
          venue: { id: 'mock-venue-1', name: 'Gor Soemantri' },
          court: { id: 'mock-court-1', name: 'Lapangan Basket 1' },
          court_id: 'mock-court-1',
          booking_date: new Date().toISOString().split('T')[0],
          start_time: '14:00',
          end_time: '16:00',
          total_price: 300000,
          status: 'WAITING_VERIFICATION',
          payment_reference: reference,
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
        } as unknown as Booking);
      }, 500);
    });
  }

  const response = await apiFetch(`${API_BASE_URL}/bookings/${id}/payment-proof`, {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ payment_reference: reference }),
  });

  if (!response.ok) {
    const errData = await response.json().catch(() => ({}));
    throw new Error(errData.message || 'Gagal mengirim bukti pembayaran');
  }

  const data = await response.json();
  return data.booking;
};

export const verifyPayment = async (id: string, isApproved: boolean, token: string): Promise<Booking> => {
  const isMock = import.meta.env.VITE_USE_MOCK_VENUE === 'true';

  if (isMock) {
    return new Promise((resolve) => {
      setTimeout(() => {
        resolve({
          id,
          customer_id: 'mock-cust-id',
          venue: { id: 'mock-venue-1', name: 'Gor Soemantri' },
          court: { id: 'mock-court-1', name: 'Lapangan Basket 1' },
          court_id: 'mock-court-1',
          booking_date: new Date().toISOString().split('T')[0],
          start_time: '14:00',
          end_time: '16:00',
          total_price: 300000,
          status: isApproved ? 'PAID' : 'PENDING_PAYMENT',
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
        } as unknown as Booking);
      }, 500);
    });
  }

  const response = await apiFetch(`${API_BASE_URL}/owner/bookings/${id}/verify-payment`, {
    method: 'PATCH',
    headers: {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ is_approved: isApproved }),
  });

  if (!response.ok) {
    const errData = await response.json().catch(() => ({}));
    throw new Error(errData.message || 'Gagal verifikasi pembayaran');
  }

  const data = await response.json();
  return data.booking;
};

export const markBookingPaid = async (id: string, token: string): Promise<Booking> => {
  const isMock = import.meta.env.VITE_USE_MOCK_VENUE === 'true';
  if (isMock) {
    return new Promise((resolve) => {
      setTimeout(() => {
        resolve({
          id,
          customer_id: 'mock-cust-id',
          venue: { id: 'mock-venue-1', name: 'Gor Soemantri' },
          court: { id: 'mock-court-1', name: 'Lapangan Basket 1' },
          court_id: 'mock-court-1',
          booking_date: new Date().toISOString().split('T')[0],
          start_time: '14:00',
          end_time: '16:00',
          total_price: 300000,
          status: 'PAID',
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
        } as unknown as Booking);
      }, 500);
    });
  }

  const response = await apiFetch(`${API_BASE_URL}/owner/bookings/${id}/mark-paid`, {
    method: 'PATCH',
    headers: { 'Authorization': `Bearer ${token}` }
  });
  if (!response.ok) {
    const errData = await response.json().catch(() => ({}));
    throw new Error(errData.message || 'Gagal menandai lunas');
  }
  const data = await response.json();
  return data.booking;
};

export const completeBooking = async (id: string, token: string): Promise<Booking> => {
  const isMock = import.meta.env.VITE_USE_MOCK_VENUE === 'true';
  if (isMock) {
    return new Promise((resolve) => {
      setTimeout(() => {
        resolve({
          id,
          customer_id: 'mock-cust-id',
          venue: { id: 'mock-venue-1', name: 'Gor Soemantri' },
          court: { id: 'mock-court-1', name: 'Lapangan Basket 1' },
          court_id: 'mock-court-1',
          booking_date: new Date().toISOString().split('T')[0],
          start_time: '14:00',
          end_time: '16:00',
          total_price: 300000,
          status: 'COMPLETED',
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
        } as unknown as Booking);
      }, 500);
    });
  }

  const response = await apiFetch(`${API_BASE_URL}/owner/bookings/${id}/complete`, {
    method: 'PATCH',
    headers: { 'Authorization': `Bearer ${token}` }
  });
  if (!response.ok) {
    const errData = await response.json().catch(() => ({}));
    throw new Error(errData.message || 'Gagal menandai selesai');
  }
  const data = await response.json();
  return data.booking;
};

export const cancelPaidBookingWithRefund = async (
  id: string,
  reason: string,
  token: string
): Promise<Booking> => {
  const isMock = import.meta.env.VITE_USE_MOCK_VENUE === 'true';
  if (isMock) {
    return new Promise((resolve) => {
      setTimeout(() => {
        resolve({
          id,
          status: 'CANCELLED',
        } as unknown as Booking);
      }, 500);
    });
  }

  const response = await apiFetch(`${API_BASE_URL}/owner/bookings/${id}/cancel-refund`, {
    method: 'PATCH',
    headers: {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ reason }),
  });

  if (!response.ok) {
    const errData = await response.json().catch(() => ({}));
    throw new Error(errData.message || 'Gagal membatalkan dan mencatat refund');
  }

  const data = await response.json();
  return data.booking;
};

// --- OWNER APIS ---

export async function fetchOwnerProfile(token: string): Promise<OwnerProfile> {
  const isMock = import.meta.env.VITE_USE_MOCK_VENUE === 'true';
  if (isMock) {
    return new Promise((resolve) => setTimeout(() => resolve({
      id: 'mock-owner', user_id: 'u1', business_name: 'PT Dummy Sports', phone_number: '08123456789', bank_name: 'BCA', bank_account_number: '1234567890', bank_account_name: 'Dummy Owner', created_at: '', updated_at: ''
    }), 500));
  }
  const response = await apiFetch(`${API_BASE_URL}/owner/profile`, {
    headers: { 'Authorization': `Bearer ${token}` }
  });
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Failed to fetch owner profile');
  }
  const data = await response.json();
  return data.profile;
}

export async function fetchOwnerVenues(token: string): Promise<Venue[]> {
  const isMock = import.meta.env.VITE_USE_MOCK_VENUE === 'true';
  if (isMock) {
    return new Promise((resolve) => setTimeout(() => resolve(MOCK_VENUES), 500));
  }
  const response = await apiFetch(`${API_BASE_URL}/owner/venues`, {
    headers: { 'Authorization': `Bearer ${token}` }
  });
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Failed to fetch owner venues');
  }
  const data = await response.json();
  return data.venues;
}

export async function fetchOwnerVenueBookings(venueId: string, token: string, date?: string, status?: string, scope?: string, page: number = 1, limit: number = 10): Promise<PaginatedResponse<OwnerBooking>> {
  const isMock = import.meta.env.VITE_USE_MOCK_VENUE === 'true';
  if (isMock) {
    return new Promise((resolve) => setTimeout(() => resolve({ data: [], page: 1, total_pages: 1, total: 0, limit: 10 }), 500));
  }
  const url = new URL(`${API_BASE_URL}/owner/venues/${venueId}/bookings`);
  if (date) url.searchParams.append('date', date);
  if (status) url.searchParams.append('status', status);
  if (scope) url.searchParams.append('scope', scope);
  url.searchParams.append('page', page.toString());
  url.searchParams.append('limit', limit.toString());
  
  const response = await apiFetch(url.toString(), {
    headers: { 'Authorization': `Bearer ${token}` }
  });
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Failed to fetch owner bookings');
  }
  const data = await response.json();
  return data;
}

export interface GlobalBookingParams {
  venue_id?: string;
  status?: string;
  scope?: string;
  start_date?: string;
  end_date?: string;
  q?: string;
  sort?: string;
  page?: number;
  limit?: number;
}

export async function fetchOwnerGlobalBookings(token: string, params: GlobalBookingParams): Promise<PaginatedResponse<OwnerBooking>> {
  const isMock = import.meta.env.VITE_USE_MOCK_VENUE === 'true';
  if (isMock) {
    return new Promise((resolve) => setTimeout(() => resolve({ data: [], page: 1, total_pages: 1, total: 0, limit: 10 }), 500));
  }
  const url = new URL(`${API_BASE_URL}/owner/bookings`);
  
  if (params.venue_id) url.searchParams.append('venue_id', params.venue_id);
  if (params.status) url.searchParams.append('status', params.status);
  if (params.scope) url.searchParams.append('scope', params.scope);
  if (params.start_date) url.searchParams.append('start_date', params.start_date);
  if (params.end_date) url.searchParams.append('end_date', params.end_date);
  if (params.q) url.searchParams.append('q', params.q);
  if (params.sort) url.searchParams.append('sort', params.sort);
  
  url.searchParams.append('page', (params.page || 1).toString());
  url.searchParams.append('limit', (params.limit || 10).toString());
  
  const response = await apiFetch(url.toString(), {
    headers: { 'Authorization': `Bearer ${token}` }
  });
  
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Failed to fetch global bookings');
  }
  
  const data = await response.json();
  return data;
}
export const fetchBookingById = async (id: string, token: string): Promise<Booking> => {
  if (import.meta.env.VITE_USE_MOCK_VENUE === 'true') {
    return new Promise((resolve) => setTimeout(() => resolve({
      id,
      customer_id: 'mock-cust-id',
      venue: { id: 'mock-venue-1', name: 'Gor Soemantri' },
      court: { id: 'mock-court-1', name: 'Lapangan Basket 1', sport_name: 'Basketball' },
      court_id: 'mock-court-1',
      booking_date: new Date().toISOString().split('T')[0],
      start_time: '14:00',
      end_time: '16:00',
      total_price: 300000,
      status: 'PENDING_PAYMENT',
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    } as unknown as Booking), 500));
  }
  const response = await apiFetch(`${API_BASE_URL}/bookings/${id}`, {
    headers: { 'Authorization': `Bearer ${token}` }
  });
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal memuat detail pesanan');
  }
  const result = await response.json();
  return result.booking;
};

export async function fetchOwnerMetrics(token: string, startDate?: string, endDate?: string): Promise<OwnerMetrics> {
  const queryParams = new URLSearchParams();
  if (startDate) queryParams.append('start_date', startDate);
  if (endDate) queryParams.append('end_date', endDate);

  const url = `${API_BASE_URL}/owner/metrics${queryParams.toString() ? '?' + queryParams.toString() : ''}`;
  const response = await apiFetch(url, {
    headers: { 'Authorization': `Bearer ${token}` }
  });
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal mengambil metrik dashboard');
  }
  const result = await response.json();
  return result.metrics;
}

export async function fetchOwnerFinanceSummary(
  token: string,
  params: { start_date?: string; end_date?: string; venue_id?: string }
): Promise<FinanceSummaryResult> {
  const queryParams = new URLSearchParams();
  if (params.start_date) queryParams.append('start_date', params.start_date);
  if (params.end_date) queryParams.append('end_date', params.end_date);
  if (params.venue_id) queryParams.append('venue_id', params.venue_id);

  const url = `${API_BASE_URL}/owner/finance/summary${queryParams.toString() ? '?' + queryParams.toString() : ''}`;
  const response = await apiFetch(url, {
    headers: { 'Authorization': `Bearer ${token}` }
  });
  
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.error || 'Gagal mengambil ringkasan keuangan');
  }
  
  return response.json();
}

import type { TransactionListResponse, CreateTransactionRequest, FinanceTransaction } from '../types/finance';

export async function fetchTransactions(
  token: string,
  params: { start_date?: string; end_date?: string; venue_id?: string; type?: string; category?: string; page?: number; limit?: number }
): Promise<TransactionListResponse> {
  const queryParams = new URLSearchParams();
  if (params.start_date) queryParams.append('start_date', params.start_date);
  if (params.end_date) queryParams.append('end_date', params.end_date);
  if (params.venue_id) queryParams.append('venue_id', params.venue_id);
  if (params.type) queryParams.append('type', params.type);
  if (params.category) queryParams.append('category', params.category);
  if (params.page) queryParams.append('page', params.page.toString());
  if (params.limit) queryParams.append('limit', params.limit.toString());

  const url = `${API_BASE_URL}/owner/finance/transactions${queryParams.toString() ? '?' + queryParams.toString() : ''}`;
  const response = await apiFetch(url, {
    headers: { 'Authorization': `Bearer ${token}` }
  });
  
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.error || 'Gagal mengambil data transaksi');
  }
  
  return response.json();
}

export async function createTransaction(token: string, data: CreateTransactionRequest): Promise<FinanceTransaction> {
  const response = await apiFetch(`${API_BASE_URL}/owner/finance/transactions`, {
    method: 'POST',
    headers: { 
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(data)
  });
  
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.error || 'Gagal menambah transaksi');
  }
  
  return response.json();
}

export async function deleteTransaction(token: string, id: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/owner/finance/transactions/${id}`, {
    method: 'DELETE',
    headers: { 'Authorization': `Bearer ${token}` }
  });
  
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.error || 'Gagal menghapus transaksi');
  }
}

import type { BookingsTrendResponse, RevenueResponse, StatusResponse, ExpensesResponse } from '../types/analytics';

export async function fetchAnalyticsBookingsTrend(token: string, params: { start_date?: string; end_date?: string; venue_id?: string }): Promise<BookingsTrendResponse> {
  const queryParams = new URLSearchParams();
  if (params.start_date) queryParams.append('start_date', params.start_date);
  if (params.end_date) queryParams.append('end_date', params.end_date);
  if (params.venue_id) queryParams.append('venue_id', params.venue_id);

  const url = `${API_BASE_URL}/owner/analytics/bookings${queryParams.toString() ? '?' + queryParams.toString() : ''}`;
  const response = await apiFetch(url, { headers: { 'Authorization': `Bearer ${token}` } });
  
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.error || 'Gagal mengambil data analitik');
  }
  
  return response.json();
}

export async function fetchAnalyticsRevenueTrend(token: string, params: { start_date?: string; end_date?: string; venue_id?: string }): Promise<RevenueResponse> {
  const queryParams = new URLSearchParams();
  if (params.start_date) queryParams.append('start_date', params.start_date);
  if (params.end_date) queryParams.append('end_date', params.end_date);
  if (params.venue_id) queryParams.append('venue_id', params.venue_id);

  const url = `${API_BASE_URL}/owner/analytics/revenue${queryParams.toString() ? '?' + queryParams.toString() : ''}`;
  const response = await apiFetch(url, { headers: { 'Authorization': `Bearer ${token}` } });
  
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.error || 'Gagal mengambil data analitik');
  }
  
  return response.json();
}

export async function fetchAnalyticsStatusBreakdown(token: string, params: { start_date?: string; end_date?: string; venue_id?: string }): Promise<StatusResponse> {
  const queryParams = new URLSearchParams();
  if (params.start_date) queryParams.append('start_date', params.start_date);
  if (params.end_date) queryParams.append('end_date', params.end_date);
  if (params.venue_id) queryParams.append('venue_id', params.venue_id);

  const url = `${API_BASE_URL}/owner/analytics/status${queryParams.toString() ? '?' + queryParams.toString() : ''}`;
  const response = await apiFetch(url, { headers: { 'Authorization': `Bearer ${token}` } });
  
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.error || 'Gagal mengambil data analitik');
  }
  
  return response.json();
}

export async function fetchAnalyticsExpensesBreakdown(token: string, params: { start_date?: string; end_date?: string; venue_id?: string }): Promise<ExpensesResponse> {
  const queryParams = new URLSearchParams();
  if (params.start_date) queryParams.append('start_date', params.start_date);
  if (params.end_date) queryParams.append('end_date', params.end_date);
  if (params.venue_id) queryParams.append('venue_id', params.venue_id);

  const url = `${API_BASE_URL}/owner/analytics/expenses${queryParams.toString() ? '?' + queryParams.toString() : ''}`;
  const response = await apiFetch(url, { headers: { 'Authorization': `Bearer ${token}` } });
  
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.error || 'Gagal mengambil data analitik');
  }
  
  return response.json();
}

import type { RefundRequest, PaginatedOwnerRefundRequests } from '../types/refund';

export async function createRefundRequest(bookingId: string, reason: string, token: string): Promise<RefundRequest> {
  const response = await apiFetch(`${API_BASE_URL}/bookings/${bookingId}/refund-request`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`
    },
    body: JSON.stringify({ reason })
  });

  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal mengajukan refund');
  }

  const data = await response.json();
  return data.refund_request;
}

export async function fetchRefundRequestByBooking(bookingId: string, token: string): Promise<RefundRequest | null> {
  const response = await apiFetch(`${API_BASE_URL}/bookings/${bookingId}/refund-request`, {
    headers: {
      'Authorization': `Bearer ${token}`
    }
  });

  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal mengambil status refund');
  }

  const result = await response.json();
  return result.data;
}

export async function fetchOwnerRefundRequests(
  token: string, 
  page = 1, 
  limit = 10,
  status = '',
  venueId = ''
): Promise<PaginatedOwnerRefundRequests> {
  const queryParams = new URLSearchParams({
    page: page.toString(),
    limit: limit.toString(),
  });
  if (status) queryParams.append('status', status);
  if (venueId) queryParams.append('venue_id', venueId);

  const response = await apiFetch(`${API_BASE_URL}/owner/refund-requests?${queryParams.toString()}`, {
    headers: {
      'Authorization': `Bearer ${token}`
    }
  });

  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal mengambil daftar permintaan refund');
  }

  return response.json();
}

export async function approveRefundRequest(id: string, ownerNote: string, token: string): Promise<{ message: string }> {
  const response = await apiFetch(`${API_BASE_URL}/owner/refund-requests/${id}/approve`, {
    method: 'PATCH',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`
    },
    body: JSON.stringify({ owner_note: ownerNote })
  });

  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal menyetujui refund');
  }

  return response.json();
}

export async function rejectRefundRequest(id: string, ownerNote: string, token: string): Promise<{ message: string }> {
  const response = await apiFetch(`${API_BASE_URL}/owner/refund-requests/${id}/reject`, {
    method: 'PATCH',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`
    },
    body: JSON.stringify({ owner_note: ownerNote })
  });

  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || 'Gagal menolak refund');
  }

  return response.json();
}
