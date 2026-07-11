import React from 'react';
import { Navigate } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';
import { getRoleHomeRoute } from '../lib/roleRouting';
import { AdminLayout } from './admin/AdminLayout';

export const SuperAdminRoute: React.FC = () => {
  const { user, isLoading } = useAuth();

  if (isLoading) {
    return (
      <div className="flex h-screen w-screen items-center justify-center bg-slate-50">
        <div className="flex flex-col items-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-emerald-500 border-t-transparent"></div>
          <p className="mt-4 text-sm font-medium text-slate-500">Loading...</p>
        </div>
      </div>
    );
  }

  if (!user || user.role !== 'SUPER_ADMIN') {
    const redirectPath = getRoleHomeRoute(user?.role);
    return <Navigate to={redirectPath} replace />;
  }

  return <AdminLayout />;
};
