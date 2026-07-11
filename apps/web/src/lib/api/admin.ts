import { apiFetch, API_BASE_URL } from '../api';

export interface PaginationQuery {
  page?: number;
  limit?: number;
}

export interface UserQuery extends PaginationQuery {
  search?: string;
  role?: string;
  status?: string;
}

export interface UserResponse {
  id: string;
  name: string;
  email: string;
  phone?: string;
  role: string;
  status: string;
  created_at: string;
}

export interface OwnerQuery extends PaginationQuery {
  search?: string;
  status?: string;
}

export interface OwnerResponse {
  id: string;
  user_id: string;
  business_name: string;
  status: string;
  created_at: string;
}

export interface VenueQuery extends PaginationQuery {
  search?: string;
  status?: string;
}

export interface VenueResponse {
  id: string;
  owner_profile_id: string;
  name: string;
  city: string;
  status: string;
  created_at: string;
}

export interface AuditLogQuery extends PaginationQuery {
  action?: string;
  entity_type?: string;
}

export interface AuditLogResponse {
  id: string;
  owner_profile_id?: string;
  actor_user_id?: string;
  actor_role: string;
  action: string;
  entity_type: string;
  entity_id?: string;
  metadata: any;
  ip_address?: string;
  user_agent?: string;
  created_at: string;
}

export interface PaginatedResponse<T> {
  data: T[];
  total_items: number;
  total_pages: number;
  page: number;
  limit: number;
}

export interface DashboardStatsResponse {
  total_users: number;
  total_owners: number;
  total_venues: number;
  total_bookings: number;
}

const ADMIN_REQUEST_TIMEOUT_MS = 10000;

const mutationDeadline = (): string => String(Date.now() + ADMIN_REQUEST_TIMEOUT_MS);

export const adminApi = {
  getUsers: async (params?: UserQuery): Promise<PaginatedResponse<UserResponse>> => {
    const query = new URLSearchParams(params as any).toString();
    const token = localStorage.getItem('auth_token');
    const response = await apiFetch(`${API_BASE_URL}/admin/users?${query}`, {
      headers: { 'Authorization': `Bearer ${token}` }
    });
    if (!response.ok) throw new Error('Failed to fetch users');
    return response.json();
  },
  
  getOwners: async (params?: OwnerQuery): Promise<PaginatedResponse<OwnerResponse>> => {
    const query = new URLSearchParams(params as any).toString();
    const token = localStorage.getItem('auth_token');
    const response = await apiFetch(`${API_BASE_URL}/admin/owners?${query}`, {
      headers: { 'Authorization': `Bearer ${token}` }
    });
    if (!response.ok) throw new Error('Failed to fetch owners');
    return response.json();
  },

  updateOwnerStatus: async (id: string, status: 'ACTIVE' | 'SUSPENDED'): Promise<void> => {
    const token = localStorage.getItem('auth_token');
    const response = await apiFetch(`${API_BASE_URL}/admin/owners/${id}/status`, {
      method: 'PATCH',
      timeoutMs: ADMIN_REQUEST_TIMEOUT_MS,
      headers: { 
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json',
        'X-Request-Deadline-Ms': mutationDeadline()
      },
      body: JSON.stringify({ status })
    });
    if (!response.ok) throw new Error('Failed to update owner status');
  },

  getVenues: async (params?: VenueQuery): Promise<PaginatedResponse<VenueResponse>> => {
    const query = new URLSearchParams(params as any).toString();
    const token = localStorage.getItem('auth_token');
    const response = await apiFetch(`${API_BASE_URL}/admin/venues?${query}`, {
      headers: { 'Authorization': `Bearer ${token}` }
    });
    if (!response.ok) throw new Error('Failed to fetch venues');
    return response.json();
  },

  updateVenueStatus: async (id: string, status: 'ACTIVE' | 'SUSPENDED'): Promise<void> => {
    const token = localStorage.getItem('auth_token');
    const response = await apiFetch(`${API_BASE_URL}/admin/venues/${id}/status`, {
      method: 'PATCH',
      timeoutMs: ADMIN_REQUEST_TIMEOUT_MS,
      headers: { 
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json',
        'X-Request-Deadline-Ms': mutationDeadline()
      },
      body: JSON.stringify({ status })
    });
    if (!response.ok) throw new Error('Failed to update venue status');
  },

  getAuditLogs: async (params?: AuditLogQuery): Promise<PaginatedResponse<AuditLogResponse>> => {
    const query = new URLSearchParams(params as any).toString();
    const token = localStorage.getItem('auth_token');
    const response = await apiFetch(`${API_BASE_URL}/admin/audit-logs?${query}`, {
      headers: { 'Authorization': `Bearer ${token}` }
    });
    if (!response.ok) throw new Error('Failed to fetch audit logs');
    return response.json();
  },

  getDashboardStats: async (): Promise<DashboardStatsResponse> => {
    const token = localStorage.getItem('auth_token');
    const response = await apiFetch(`${API_BASE_URL}/admin/dashboard`, {
      timeoutMs: ADMIN_REQUEST_TIMEOUT_MS,
      headers: { 'Authorization': `Bearer ${token}` }
    });
    if (!response.ok) {
      throw new Error('Failed to fetch dashboard stats');
    }
    return response.json();
  }
};
