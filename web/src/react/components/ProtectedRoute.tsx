import React from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';
import PokeballLoader from '../PokeballLoader';

interface ProtectedRouteProps {
  children: React.ReactNode;
}

/**
 * ProtectedRoute component that redirects to login if user is not authenticated.
 * Preserves the attempted location so user can be redirected back after login.
 */
export function ProtectedRoute({ children }: ProtectedRouteProps) {
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
    // Redirect to login page with the current location in state
    // so we can redirect back after successful login
    return <Navigate to="/login" state={{ from: location }} replace />;
  }

  return <>{children}</>;
}

export default ProtectedRoute;
