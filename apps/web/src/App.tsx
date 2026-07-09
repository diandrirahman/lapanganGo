import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { AuthProvider } from './contexts/AuthContext';
import { Toaster } from 'react-hot-toast';
import { HomePage } from './pages/HomePage';
import { LoginPage } from './pages/LoginPage';
import { RegisterPage } from './pages/RegisterPage';
import { VenuesSearchPage } from './pages/VenuesSearchPage';
import { VenueDetailPage } from './pages/VenueDetailPage';
import { CourtAvailabilityPage } from './pages/CourtAvailabilityPage';
import { CustomerBookingsPage } from './pages/CustomerBookingsPage';
import { CustomerBookingDetailPage } from './pages/CustomerBookingDetailPage';
import { OpenMatchesPage } from './pages/OpenMatchesPage';
import { MabarDetailPage } from './pages/MabarDetailPage';
import { NotFoundPage } from './pages/NotFoundPage';
import { StaffSetupPasswordPage } from './pages/StaffSetupPasswordPage';
import { ProtectedRoute } from './components/ProtectedRoute';
import { SuperAdminRoute } from './components/SuperAdminRoute';

// Admin Pages
import { AdminUsersPage } from './pages/admin/AdminUsersPage';
import { AdminOwnersPage } from './pages/admin/AdminOwnersPage';
import { AdminVenuesPage } from './pages/admin/AdminVenuesPage';
import { AdminAuditLogsPage } from './pages/admin/AdminAuditLogsPage';
import { AdminDashboardPage } from './pages/admin/AdminDashboardPage';

// Owner Pages
import { OwnerDashboardPage } from './pages/owner/OwnerDashboardPage';
import { OwnerVenuesPage } from './pages/owner/OwnerVenuesPage';
import { CreateVenuePage } from './pages/owner/CreateVenuePage';
import { EditVenuePage } from './pages/owner/EditVenuePage';
import { OwnerCourtsPage } from './pages/owner/OwnerCourtsPage';
import { OwnerVenueBookingsPage } from './pages/owner/OwnerVenueBookingsPage';
import { OwnerBookingsPage } from './pages/owner/OwnerBookingsPage';
import { OwnerRefundsPage } from './pages/owner/OwnerRefundsPage';
import { OwnerFinancePage } from './pages/owner/OwnerFinancePage';
import { OwnerPromosPage } from './pages/owner/OwnerPromosPage';
import { OwnerStaffPage } from './pages/owner/OwnerStaffPage';
import { OwnerAuditLogsPage } from './pages/owner/OwnerAuditLogsPage';

function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Toaster
          position="top-center"
          toastOptions={{
            duration: 3500,
            style: {
              borderRadius: '14px',
              background: '#0F172A',
              color: '#FFFFFF',
              fontWeight: 700,
              boxShadow: '0 20px 40px rgba(15, 23, 42, 0.18)',
              maxWidth: 'calc(100vw - 32px)',
            },
            success: {
              duration: 3000,
            },
            error: {
              duration: 4500,
            },
          }}
        />
        <Routes>
          <Route path="/" element={<HomePage />} />
          <Route path="/login" element={<LoginPage />} />
          <Route path="/register" element={<RegisterPage />} />
          <Route path="/staff/setup-password" element={<StaffSetupPasswordPage />} />
          <Route path="/venues" element={<VenuesSearchPage />} />
          <Route path="/venues/:id" element={<VenueDetailPage />} />
          <Route path="/venues/:venueId/courts/:courtId/availability" element={<CourtAvailabilityPage />} />
          <Route path="/courts/:courtId/availability" element={<CourtAvailabilityPage />} />
          <Route path="/open-matches" element={<OpenMatchesPage />} />
          <Route path="/open-matches/:id" element={<MabarDetailPage />} />

          <Route element={<ProtectedRoute requiredRole="CUSTOMER" />}>
            <Route path="/bookings" element={<CustomerBookingsPage />} />
            <Route path="/bookings/:id" element={<CustomerBookingDetailPage />} />
          </Route>

          {/* Owner Routes */}
          <Route element={<ProtectedRoute requiredRole="OWNER" />}>
            <Route path="/owner/dashboard" element={<OwnerDashboardPage />} />

            <Route element={<ProtectedRoute requiredRole="OWNER" requiredPermission="BOOKINGS_READ" />}>
              <Route path="/owner/bookings" element={<OwnerBookingsPage />} />
              <Route path="/owner/venues/:id/bookings" element={<OwnerVenueBookingsPage />} />
            </Route>

            <Route element={<ProtectedRoute requiredRole="OWNER" requiredPermission="REFUNDS_READ" />}>
              <Route path="/owner/refunds" element={<OwnerRefundsPage />} />
            </Route>

            <Route element={<ProtectedRoute requiredRole="OWNER" requiredPermission="FINANCE_READ" />}>
              <Route path="/owner/finance" element={<OwnerFinancePage />} />
            </Route>

            <Route element={<ProtectedRoute requiredRole="OWNER" requiredPermission="VENUES_READ" />}>
              <Route path="/owner/venues" element={<OwnerVenuesPage />} />
              <Route path="/owner/venues/new" element={<CreateVenuePage />} />
              <Route path="/owner/venues/:id/edit" element={<EditVenuePage />} />
              <Route path="/owner/venues/:id/courts" element={<OwnerCourtsPage />} />
            </Route>

            <Route element={<ProtectedRoute requiredRole="OWNER" requiredPermission="PROMOS_READ" />}>
              <Route path="/owner/promos" element={<OwnerPromosPage />} />
            </Route>

            {/* Staff Management - Owner only */}
            <Route element={<ProtectedRoute requiredRole="OWNER" requireActualOwner={true} />}>
              <Route path="/owner/staff" element={<OwnerStaffPage />} />
              <Route path="/owner/audit-logs" element={<OwnerAuditLogsPage />} />
            </Route>
          </Route>

          {/* Admin Routes */}
          <Route element={<SuperAdminRoute />}>
            <Route path="/admin" element={<Navigate to="/admin/dashboard" replace />} />
            <Route path="/admin/dashboard" element={<AdminDashboardPage />} />
            <Route path="/admin/users" element={<AdminUsersPage />} />
            <Route path="/admin/owners" element={<AdminOwnersPage />} />
            <Route path="/admin/venues" element={<AdminVenuesPage />} />
            <Route path="/admin/audit-logs" element={<AdminAuditLogsPage />} />
          </Route>

          {/* Catch-All / 404 */}
          <Route path="*" element={<NotFoundPage />} />
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  );
}

export default App;
