import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { SuperAdminRoute } from '../components/SuperAdminRoute';
import { useAuth } from '../contexts/AuthContext';

vi.mock('../contexts/AuthContext', () => ({ useAuth: vi.fn() }));
vi.mock('../components/admin/AdminLayout', () => ({ AdminLayout: () => <div data-testid="admin-layout">Admin finance shell</div> }));

const authMock = vi.mocked(useAuth);
const baseAuth = {
  token: null,
  isAuthenticated: false,
  isLoading: false,
  login: vi.fn(),
  logout: vi.fn(),
  isActualOwner: () => false,
  hasOwnerPermission: () => false,
  isWorkspaceUser: () => false,
};

describe('SuperAdminRoute role matrix', () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it.each([
    ['anonymous', null, false],
    ['customer', { role: 'CUSTOMER', status: 'ACTIVE' }, false],
    ['owner', { role: 'OWNER', status: 'ACTIVE' }, false],
    ['staff', { role: 'STAFF', status: 'ACTIVE' }, false],
    ['suspended superadmin', { role: 'SUPER_ADMIN', status: 'SUSPENDED' }, false],
    ['active superadmin', { role: 'SUPER_ADMIN', status: 'ACTIVE' }, true],
  ])('%s is handled by the UI route guard', (_name, user, allowed) => {
    authMock.mockReturnValue({ ...baseAuth, user: user ? { ...user, id: 'qa-user', name: 'QA User', email: 'qa@example.com', created_at: '2026-07-18T00:00:00Z' } : null, isAuthenticated: Boolean(user) });
    render(<MemoryRouter initialEntries={['/admin/finance']}><Routes><Route path="/admin/finance" element={<SuperAdminRoute />} /><Route path="*" element={<div data-testid="redirected">Redirected</div>} /></Routes></MemoryRouter>);

    if (allowed) {
      expect(screen.getByTestId('admin-layout')).toBeTruthy();
      expect(screen.queryByTestId('redirected')).toBeNull();
    } else {
      expect(screen.getByTestId('redirected')).toBeTruthy();
      expect(screen.queryByTestId('admin-layout')).toBeNull();
    }
  });
});
