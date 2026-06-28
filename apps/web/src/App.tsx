import { BrowserRouter, Routes, Route } from 'react-router-dom';
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
import { ProtectedRoute } from './components/ProtectedRoute';

// Owner Pages
import { OwnerDashboardPage } from './pages/owner/OwnerDashboardPage';
import { OwnerVenuesPage } from './pages/owner/OwnerVenuesPage';
import { CreateVenuePage } from './pages/owner/CreateVenuePage';
import { EditVenuePage } from './pages/owner/EditVenuePage';
import { OwnerCourtsPage } from './pages/owner/OwnerCourtsPage';
import { OwnerVenueBookingsPage } from './pages/owner/OwnerVenueBookingsPage';

function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<HomePage />} />
          <Route path="/login" element={<LoginPage />} />
          <Route path="/register" element={<RegisterPage />} />
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
            <Route path="/owner/venues" element={<OwnerVenuesPage />} />
            <Route path="/owner/venues/new" element={<CreateVenuePage />} />
            <Route path="/owner/venues/:id/edit" element={<EditVenuePage />} />
            <Route path="/owner/venues/:id/courts" element={<OwnerCourtsPage />} />
            <Route path="/owner/venues/:id/bookings" element={<OwnerVenueBookingsPage />} />
          </Route>

          {/* Catch-All / 404 */}
          <Route path="*" element={<NotFoundPage />} />
        </Routes>
      </BrowserRouter>
      <Toaster position="bottom-right" />
    </AuthProvider>
  );
}

export default App;
