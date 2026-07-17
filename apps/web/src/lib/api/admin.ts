import { apiFetch, API_BASE_URL } from '../api';
import type {
  CreatePlatformExpenseRequest,
  FinanceApiErrorBody,
  PlatformExpense,
  PlatformExpenseQuery,
  PlatformJournal,
  PlatformJournalQuery,
} from '../../types/platformExpense';

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
  scope?: AuditScope;
  action?: string;
  entity_type?: string;
}

export type AuditScope = 'OWNER' | 'PLATFORM' | 'ALL';

export interface AuditLogResponse {
  id: string;
  scope: AuditScope;
  owner_profile_id?: string;
  actor_user_id?: string;
  actor_role: string;
  action: string;
  entity_type: string;
  entity_id?: string;
  venue_id?: string;
  metadata: unknown;
  created_at: string;
}

export type CommercialTermScope = 'ALL' | 'GLOBAL' | 'OWNER';
export type CommercialTermStatus = 'CURRENT' | 'SCHEDULED' | 'HISTORICAL';
export type CommercialTermPhase = 'TRIAL' | 'INTRODUCTORY' | 'STANDARD' | 'CUSTOM';
export type CommercialTermFinanceMode = 'SIMULATION' | 'LIVE';
export type CommercialTermCollectionMethod = 'NONE' | 'DEDUCT_FROM_PAYOUT';

export interface CommercialTermsQuery extends PaginationQuery {
  scope?: CommercialTermScope;
  owner_profile_id?: string;
  status?: CommercialTermStatus;
}

export interface CommercialTermResponse {
  id: string;
  owner_profile_id: string | null;
  scope_key: string;
  label: string;
  phase: CommercialTermPhase;
  finance_mode: CommercialTermFinanceMode;
  collection_method: CommercialTermCollectionMethod;
  commission_bps: number;
  valid_from: string;
  valid_until: string | null;
  supersedes_id: string | null;
  created_by_user_id: string;
  created_at: string;
  status: CommercialTermStatus;
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

export type PlatformExpensePage = PaginatedResponse<PlatformExpense>;
export type PlatformJournalPage = PaginatedResponse<PlatformJournal>;

export class AdminApiError extends Error {
  readonly status: number;
  readonly body: FinanceApiErrorBody;

  constructor(status: number, body: FinanceApiErrorBody, fallback: string) {
    super(body.message || fallback);
    this.name = 'AdminApiError';
    this.status = status;
    this.body = body;
  }
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

  getAuditLogs: async (
    params?: AuditLogQuery,
    options?: { signal?: AbortSignal },
  ): Promise<PaginatedResponse<AuditLogResponse>> => {
    const searchParams = new URLSearchParams();
    Object.entries(params ?? {}).forEach(([key, value]) => {
      if (value !== undefined && value !== '') {
        searchParams.set(key, String(value));
      }
    });
    const token = localStorage.getItem('auth_token');
    const query = searchParams.toString();
    const response = await apiFetch(`${API_BASE_URL}/admin/audit-logs${query ? `?${query}` : ''}`, {
      signal: options?.signal,
      timeoutMs: ADMIN_REQUEST_TIMEOUT_MS,
      headers: { 'Authorization': `Bearer ${token}` },
    });
    if (!response.ok) throw new Error('Failed to fetch audit logs');
    return response.json();
  },

  getCommercialTerms: async (
    params?: CommercialTermsQuery,
    options?: { signal?: AbortSignal },
  ): Promise<PaginatedResponse<CommercialTermResponse>> => {
    const searchParams = new URLSearchParams();
    Object.entries(params ?? {}).forEach(([key, value]) => {
      if (value !== undefined && value !== '') {
        searchParams.set(key, String(value));
      }
    });

    const token = localStorage.getItem('auth_token');
    const query = searchParams.toString();
    const response = await apiFetch(`${API_BASE_URL}/admin/commercial-terms${query ? `?${query}` : ''}`, {
      signal: options?.signal,
      timeoutMs: ADMIN_REQUEST_TIMEOUT_MS,
      headers: { 'Authorization': `Bearer ${token}` },
    });
    if (!response.ok) throw new Error('Failed to fetch commercial terms');
    return response.json();
  },

  getPlatformExpenses: async (
    params?: PlatformExpenseQuery,
    options?: { signal?: AbortSignal },
  ): Promise<PlatformExpensePage> => {
    const searchParams = new URLSearchParams();
    Object.entries(params ?? {}).forEach(([key, value]) => {
      if (value !== undefined && value !== '') searchParams.set(key, String(value));
    });
    const token = localStorage.getItem('auth_token');
    const query = searchParams.toString();
    const response = await apiFetch(`${API_BASE_URL}/admin/finance/expenses${query ? `?${query}` : ''}`, {
      signal: options?.signal,
      timeoutMs: ADMIN_REQUEST_TIMEOUT_MS,
      headers: { Authorization: `Bearer ${token}` },
    });
    if (!response.ok) {
      const body = await response.json().catch(() => ({}));
      throw new AdminApiError(response.status, body, 'Platform expenses could not be loaded');
    }
    return response.json();
  },

  getPlatformJournals: async (
    params?: PlatformJournalQuery,
    options?: { signal?: AbortSignal },
  ): Promise<PlatformJournalPage> => {
    const searchParams = new URLSearchParams();
    Object.entries(params ?? {}).forEach(([key, value]) => {
      if (value !== undefined && value !== '') searchParams.set(key, String(value));
    });
    const token = localStorage.getItem('auth_token');
    const query = searchParams.toString();
    const response = await apiFetch(`${API_BASE_URL}/admin/finance/journals${query ? `?${query}` : ''}`, {
      signal: options?.signal,
      timeoutMs: ADMIN_REQUEST_TIMEOUT_MS,
      headers: { Authorization: `Bearer ${token}` },
    });
    if (!response.ok) {
      const body = await response.json().catch(() => ({}));
      throw new AdminApiError(response.status, body, 'Platform journals could not be loaded');
    }
    return response.json();
  },

  createPlatformExpense: async (
    request: CreatePlatformExpenseRequest,
    idempotencyKey: string,
    options?: { signal?: AbortSignal },
  ): Promise<PlatformExpense> => {
    const token = localStorage.getItem('auth_token');
    const response = await apiFetch(`${API_BASE_URL}/admin/finance/expenses`, {
      method: 'POST',
      signal: options?.signal,
      timeoutMs: ADMIN_REQUEST_TIMEOUT_MS,
      headers: {
        Authorization: `Bearer ${token}`,
        'Content-Type': 'application/json',
        'Idempotency-Key': idempotencyKey,
        'X-Request-Deadline-Ms': mutationDeadline(),
      },
      body: JSON.stringify(request),
    });
    if (!response.ok) {
      const body = await response.json().catch(() => ({}));
      throw new AdminApiError(response.status, body, 'Platform expense could not be created');
    }
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
