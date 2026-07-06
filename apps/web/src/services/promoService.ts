import { API_BASE_URL, apiFetch } from '../lib/api';
import type { Promo, CreatePromoRequest } from '../types/promo';

export const promoService = {
  async getPromos(token: string): Promise<Promo[]> {
    const res = await apiFetch(`${API_BASE_URL}/owner/promos`, {
      headers: { Authorization: `Bearer ${token}` }
    });
    if (!res.ok) throw new Error('Failed to fetch promos');
    return res.json();
  },

  async getPromo(token: string, id: string): Promise<Promo> {
    const res = await apiFetch(`${API_BASE_URL}/owner/promos/${id}`, {
      headers: { Authorization: `Bearer ${token}` }
    });
    if (!res.ok) throw new Error('Failed to fetch promo');
    return res.json();
  },

  async createPromo(token: string, data: CreatePromoRequest): Promise<Promo> {
    const res = await apiFetch(`${API_BASE_URL}/owner/promos`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`
      },
      body: JSON.stringify(data)
    });
    if (!res.ok) {
      const errorData = await res.json().catch(() => ({}));
      throw new Error(errorData.message || 'Failed to create promo');
    }
    return res.json();
  },

  async updatePromo(token: string, id: string, data: CreatePromoRequest): Promise<Promo> {
    const res = await apiFetch(`${API_BASE_URL}/owner/promos/${id}`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`
      },
      body: JSON.stringify(data)
    });
    if (!res.ok) {
      const errorData = await res.json().catch(() => ({}));
      throw new Error(errorData.message || 'Failed to update promo');
    }
    return res.json();
  },

  async togglePromo(token: string, id: string): Promise<Promo> {
    const res = await apiFetch(`${API_BASE_URL}/owner/promos/${id}/toggle`, {
      method: 'PATCH',
      headers: { Authorization: `Bearer ${token}` }
    });
    if (!res.ok) {
      const errorData = await res.json().catch(() => ({}));
      throw new Error(errorData.message || 'Failed to toggle promo status');
    }
    return res.json();
  },

  async deletePromo(token: string, id: string): Promise<void> {
    const res = await apiFetch(`${API_BASE_URL}/owner/promos/${id}`, {
      method: 'DELETE',
      headers: { Authorization: `Bearer ${token}` }
    });
    if (!res.ok) {
      const errorData = await res.json().catch(() => ({}));
      throw new Error(errorData.message || 'Failed to delete promo');
    }
  }
};
