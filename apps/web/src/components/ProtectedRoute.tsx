import React from 'react';
import { Navigate, Outlet } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';
import { PageShell } from './layout/PageShell';

interface ProtectedRouteProps {
  requiredRole?: string;
  children?: React.ReactNode;
}

export const ProtectedRoute: React.FC<ProtectedRouteProps> = ({ requiredRole, children }) => {
  const { isAuthenticated, isLoading, user } = useAuth();

  if (isLoading) {
    return (
      <PageShell>
        <div className="pt-32 pb-40 flex items-center justify-center">
          <div className="text-text-muted animate-pulse">Memuat...</div>
        </div>
      </PageShell>
    );
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }

  if (requiredRole && user?.role !== requiredRole) {
    // If a specific role is required and user doesn't have it
    // Or if it's an owner trying to access customer routes (though maybe we just let them, usually owner routes are stricter)
    // The requirement says: "Customer tidak bisa akses route owner. Owner route tetap bisa diakses owner."
    // If they aren't the required role, redirect to home or login. We'll use / for now.
    return <Navigate to="/" replace />;
  }

  // Render children if provided (for individual routes), otherwise render Outlet (for nested routes layout)
  return children ? <>{children}</> : <Outlet />;
};
