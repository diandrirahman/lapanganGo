import { beforeEach, describe, expect, it } from 'vitest';
import { PORTFOLIO_DEMO_SESSION_KEY, PORTFOLIO_DEMO_STATE_KEY } from '../demo/config';
import { portfolioDemoFetchForTest } from '../demo/fetch';
import {
  createInitialPortfolioDemoState,
  loadPortfolioDemoState,
  resetPortfolioDemoState,
  savePortfolioDemoState,
  startPortfolioDemoSession,
} from '../demo/state';

describe('portfolio demo data and network isolation', () => {
  beforeEach(() => {
    localStorage.clear();
    window.history.replaceState({}, '', '/');
  });

  it('creates the deterministic role population', () => {
    const first = createInitialPortfolioDemoState();
    const second = createInitialPortfolioDemoState();

    expect(first.users).toHaveLength(121);
    expect(first.users.filter((user) => user.role === 'SUPER_ADMIN')).toHaveLength(1);
    expect(first.users.filter((user) => user.role === 'OWNER')).toHaveLength(20);
    expect(first.users.filter((user) => user.role === 'CUSTOMER')).toHaveLength(100);
    expect(second).toEqual(first);
  });

  it('persists browser-local changes and restores the baseline on reset', () => {
    const state = createInitialPortfolioDemoState();
    state.venues[0].name = 'Nama yang diubah lokal';
    savePortfolioDemoState(state);

    expect(loadPortfolioDemoState().venues[0].name).toBe('Nama yang diubah lokal');
    localStorage.setItem(PORTFOLIO_DEMO_SESSION_KEY, '{"role":"OWNER"}');
    localStorage.setItem('auth_token', 'portfolio-demo:OWNER');

    const reset = resetPortfolioDemoState();
    expect(reset.venues[0].name).toBe('Arena Senayan Demo');
    expect(localStorage.getItem(PORTFOLIO_DEMO_SESSION_KEY)).toBeNull();
    expect(localStorage.getItem('auth_token')).toBeNull();
  });

  it.each(['CUSTOMER', 'OWNER', 'SUPER_ADMIN'] as const)('creates a complete %s demo session', (role) => {
    const session = startPortfolioDemoSession(role);
    expect(session.user.role).toBe(role);
    expect(session.token).toBe(`portfolio-demo:${role}`);
    expect(localStorage.getItem(PORTFOLIO_DEMO_SESSION_KEY)).toContain(role);
    if (role === 'OWNER') expect(session.user.owner_profile).toBeDefined();
  });

  it('mutates bookings only inside the namespaced local state', async () => {
    const initialCount = loadPortfolioDemoState().bookings.length;
    const response = await portfolioDemoFetchForTest('https://demo.lapanggo.test/bookings', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ court_id: 'court-1', booking_date: '2030-06-15', start_time: '10:00', end_time: '11:00' }),
    });

    expect(response.status).toBe(201);
    expect(loadPortfolioDemoState().bookings).toHaveLength(initialCount + 1);
    expect(localStorage.getItem(PORTFOLIO_DEMO_STATE_KEY)).not.toBeNull();
  });

  it('serves role-specific API response contracts from local data', async () => {
    startPortfolioDemoSession('OWNER');
    const meResponse = await portfolioDemoFetchForTest('http://localhost:8080/auth/me');
    const profileResponse = await portfolioDemoFetchForTest('http://localhost:8080/owner/profile');
    const promoResponse = await portfolioDemoFetchForTest('http://localhost:8080/owner/promos');

    expect((await meResponse.json()).user.role).toBe('OWNER');
    expect((await profileResponse.json()).profile.business_name).toBe('LapangGo Demo Sports 01');
    expect(await promoResponse.json()).toEqual(expect.any(Array));
  });

  it('fails closed for unknown API paths instead of using a network fallback', async () => {
    const response = await portfolioDemoFetchForTest('https://api.example.test/provider/xendit', { method: 'POST' });
    const body = await response.json();

    expect(response.status).toBe(501);
    expect(body.code).toBe('PORTFOLIO_DEMO_UNAVAILABLE');
    expect(body.message).toContain('Tidak ada request yang dikirim ke backend');
  });
});
