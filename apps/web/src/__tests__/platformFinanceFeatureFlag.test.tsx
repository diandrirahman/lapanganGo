import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import App from '../App';
import { AdminLayout } from '../components/admin/AdminLayout';

let mockIsEnabled = false;

vi.mock('../contexts/AuthContext', () => ({
  AuthProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  useAuth: () => ({
    user: { id: 'admin1', role: 'SUPER_ADMIN', name: 'Admin', email: 'admin@example.com', status: 'ACTIVE' },
    isAuthenticated: true,
    isLoading: false,
    logout: vi.fn(),
  }),
}));

vi.mock('react-router-dom', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router-dom')>();
  return {
    ...actual,
    BrowserRouter: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  };
});

vi.mock('../config/features', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../config/features')>();
  return {
    ...actual,
    get isPlatformFinanceAdminEnabled() {
      return mockIsEnabled;
    },
  };
});

// Mock ALL pages imported by App to prevent side-effects/hanging
vi.mock('../pages/HomePage', () => ({ HomePage: () => <div data-testid="page">Home</div> }));
vi.mock('../pages/LoginPage', () => ({ LoginPage: () => <div data-testid="page">Login</div> }));
vi.mock('../pages/RegisterPage', () => ({ RegisterPage: () => <div data-testid="page">Register</div> }));
vi.mock('../pages/VenuesSearchPage', () => ({ VenuesSearchPage: () => <div data-testid="page">Venues Search</div> }));
vi.mock('../pages/VenueDetailPage', () => ({ VenueDetailPage: () => <div data-testid="page">Venue Detail</div> }));
vi.mock('../pages/CourtAvailabilityPage', () => ({ CourtAvailabilityPage: () => <div data-testid="page">Court Avail</div> }));
vi.mock('../pages/CustomerBookingsPage', () => ({ CustomerBookingsPage: () => <div data-testid="page">Cust Bookings</div> }));
vi.mock('../pages/CustomerBookingDetailPage', () => ({ CustomerBookingDetailPage: () => <div data-testid="page">Cust Booking Detail</div> }));
vi.mock('../pages/OpenMatchesPage', () => ({ OpenMatchesPage: () => <div data-testid="page">Open Matches</div> }));
vi.mock('../pages/MabarDetailPage', () => ({ MabarDetailPage: () => <div data-testid="page">Mabar Detail</div> }));
vi.mock('../pages/NotFoundPage', () => ({ NotFoundPage: () => <div data-testid="page">Not Found</div> }));
vi.mock('../pages/StaffSetupPasswordPage', () => ({ StaffSetupPasswordPage: () => <div data-testid="page">Staff Setup Password</div> }));

// Admin Pages
vi.mock('../pages/admin/AdminUsersPage', () => ({ AdminUsersPage: () => <div data-testid="page">Admin Users</div> }));
vi.mock('../pages/admin/AdminOwnersPage', () => ({ AdminOwnersPage: () => <div data-testid="page">Admin Owners</div> }));
vi.mock('../pages/admin/AdminVenuesPage', () => ({ AdminVenuesPage: () => <div data-testid="page">Admin Venues</div> }));
vi.mock('../pages/admin/AdminAuditLogsPage', () => ({ AdminAuditLogsPage: () => <div data-testid="page">Admin Audit Logs</div> }));
vi.mock('../pages/admin/AdminDashboardPage', () => ({ AdminDashboardPage: () => <div data-testid="dashboard-page">LapangGo Admin</div> }));
vi.mock('../pages/admin/AdminCommercialTermsPage', () => ({ AdminCommercialTermsPage: () => <div data-testid="page">Admin Commercial Terms</div> }));
vi.mock('../pages/admin/AdminPlatformExpensesPage', () => ({ AdminPlatformExpensesPage: () => <div data-testid="expenses-page">Platform Expenses</div> }));
vi.mock('../pages/admin/AdminPlatformFinancePage', () => ({ AdminPlatformFinancePage: () => <div data-testid="finance-page">Platform Finance</div> }));

// Owner Pages
vi.mock('../pages/owner/OwnerDashboardPage', () => ({ OwnerDashboardPage: () => <div data-testid="page">Owner Dashboard</div> }));
vi.mock('../pages/owner/OwnerVenuesPage', () => ({ OwnerVenuesPage: () => <div data-testid="page">Owner Venues</div> }));
vi.mock('../pages/owner/CreateVenuePage', () => ({ CreateVenuePage: () => <div data-testid="page">Create Venue</div> }));
vi.mock('../pages/owner/EditVenuePage', () => ({ EditVenuePage: () => <div data-testid="page">Edit Venue</div> }));
vi.mock('../pages/owner/OwnerCourtsPage', () => ({ OwnerCourtsPage: () => <div data-testid="page">Owner Courts</div> }));
vi.mock('../pages/owner/OwnerVenueBookingsPage', () => ({ OwnerVenueBookingsPage: () => <div data-testid="page">Owner Venue Bookings</div> }));
vi.mock('../pages/owner/OwnerBookingsPage', () => ({ OwnerBookingsPage: () => <div data-testid="page">Owner Bookings</div> }));
vi.mock('../pages/owner/OwnerRefundsPage', () => ({ OwnerRefundsPage: () => <div data-testid="page">Owner Refunds</div> }));
vi.mock('../pages/owner/OwnerFinancePage', () => ({ OwnerFinancePage: () => <div data-testid="page">Owner Finance</div> }));
vi.mock('../pages/owner/OwnerPromosPage', () => ({ OwnerPromosPage: () => <div data-testid="page">Owner Promos</div> }));
vi.mock('../pages/owner/OwnerStaffPage', () => ({ OwnerStaffPage: () => <div data-testid="page">Owner Staff</div> }));
vi.mock('../pages/owner/OwnerAuditLogsPage', () => ({ OwnerAuditLogsPage: () => <div data-testid="page">Owner Audit Logs</div> }));

describe('Platform Finance Admin Feature Flag', () => {

  describe('parsePlatformFinanceAdminFlag', () => {
    it('returns false for undefined, empty, or false strings', async () => {
      const { parsePlatformFinanceAdminFlag } = await import('../config/features');
      expect(parsePlatformFinanceAdminFlag(undefined)).toBe(false);
      expect(parsePlatformFinanceAdminFlag('')).toBe(false);
      expect(parsePlatformFinanceAdminFlag('false')).toBe(false);
    });

    it('returns true only for exact string "true"', async () => {
      const { parsePlatformFinanceAdminFlag } = await import('../config/features');
      expect(parsePlatformFinanceAdminFlag('true')).toBe(true);
      expect(parsePlatformFinanceAdminFlag('TRUE')).toBe(false);
      expect(parsePlatformFinanceAdminFlag('1')).toBe(false);
    });
  });

  describe('Routing behavior when disabled', () => {
    beforeEach(() => {
      mockIsEnabled = false;
    });

    it('redirects /admin/finance to /admin/dashboard', () => {
      render(
        <MemoryRouter initialEntries={['/admin/finance']}>
          <App />
        </MemoryRouter>
      );
      
      expect(screen.getAllByText(/LapangGo Admin/i).length).toBeGreaterThan(0);
      expect(screen.queryByText(/Platform Finance/i)).toBeNull();
    });

    it('redirects /admin/finance/expenses to /admin/dashboard', () => {
      render(
        <MemoryRouter initialEntries={['/admin/finance/expenses']}>
          <App />
        </MemoryRouter>
      );
      
      expect(screen.getAllByText(/LapangGo Admin/i).length).toBeGreaterThan(0);
      expect(screen.queryByText(/Platform Expenses/i)).toBeNull();
    });

    it('redirects /admin/finance/unknown to /admin/dashboard', () => {
      render(
        <MemoryRouter initialEntries={['/admin/finance/unknown']}>
          <App />
        </MemoryRouter>
      );
      
      expect(screen.getAllByText(/LapangGo Admin/i).length).toBeGreaterThan(0);
    });

    it('hides Platform Finance from navigation menu', () => {
      render(
        <MemoryRouter initialEntries={['/admin/dashboard']}>
          <AdminLayout />
        </MemoryRouter>
      );
      
      expect(screen.getAllByText('Users').length).toBeGreaterThan(0);
      expect(screen.queryByText('Platform Finance')).toBeNull();
    });
  });

  describe('Routing behavior when enabled', () => {
    beforeEach(() => {
      mockIsEnabled = true;
    });

    it('shows Platform Finance in navigation menu', () => {
      render(
        <MemoryRouter initialEntries={['/admin/dashboard']}>
          <AdminLayout />
        </MemoryRouter>
      );
      
      expect(screen.getAllByText('Platform Finance').length).toBeGreaterThan(0);
    });
  });
});
