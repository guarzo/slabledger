import React from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';
import PokeballLoader from '../PokeballLoader';

interface AdminRouteProps {
  children: React.ReactNode;
}

/**
 * AdminRoute - Requires both authentication and admin privileges.
 * Redirects to login if not authenticated, to home if not admin.
 */
export default function AdminRoute({ children }: AdminRouteProps) {
  const { user, loading } = useAuth();
  const location = useLocation();

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-[50vh]">
        <div className="text-center">
          <PokeballLoader />
          <p className="mt-4 text-sm text-[var(--text-muted)]">Checking authentication...</p>
        </div>
      </div>
    );
  }

  if (!user) {
    return <Navigate to="/login" state={{ from: location }} replace />;
  }

  if (!user.is_admin) {
    return <Navigate to="/" replace />;
  }

  return <>{children}</>;
}
