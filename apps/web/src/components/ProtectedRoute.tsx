import React from 'react';
import { Navigate, Outlet } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';
import { PageShell } from './layout/PageShell';

interface ProtectedRouteProps {
  requiredRole?: string;
  requiredPermission?: string;
  requireActualOwner?: boolean;
  children?: React.ReactNode;
}

export const ProtectedRoute: React.FC<ProtectedRouteProps> = ({
  requiredRole,
  requiredPermission,
  requireActualOwner,
  children
}) => {
  const { isAuthenticated, isLoading, user, isWorkspaceUser, hasOwnerPermission, isActualOwner } = useAuth();

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

  // Handle owner workspace routes
  if (requiredRole === 'OWNER') {
    if (!isWorkspaceUser()) {
      return <Navigate to="/" replace />;
    }

    if (requireActualOwner && !isActualOwner()) {
      return <Navigate to="/owner/dashboard" replace />;
    }

    if (requiredPermission && !hasOwnerPermission(requiredPermission)) {
      return <Navigate to="/owner/dashboard" replace />;
    }
  } else if (requiredRole && user?.role !== requiredRole) {
    const redirectPath = isWorkspaceUser() ? '/owner/dashboard' : '/';
    return <Navigate to={redirectPath} replace />;
  }

  // Render children if provided (for individual routes), otherwise render Outlet (for nested routes layout)
  return children ? <>{children}</> : <Outlet />;
};
