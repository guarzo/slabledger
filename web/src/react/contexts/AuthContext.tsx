import React, { createContext, useContext, useState, useEffect, useCallback, useRef, ReactNode } from 'react';
import { api } from '../../js/api';
import { reportError } from '../../js/errors';

interface User {
  id: number;
  username: string;
  email: string;
  avatar_url: string;
  is_admin: boolean;
  last_login_at: string | null;
}

interface AuthContextType {
  user: User | null;
  loading: boolean;
  login: () => void;
  logout: () => Promise<void>;
  refetchUser: () => Promise<void>;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

interface AuthProviderProps {
  children: ReactNode;
  onIdentityChange?: (id: number | null) => void;
}

export const AuthProvider: React.FC<AuthProviderProps> = ({ children, onIdentityChange }) => {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const requestId = useRef(0);

  const fetchCurrentUser = useCallback(async () => {
    const request = ++requestId.current;
    try {
      const userData = await api.get<User>('/auth/user');
      if (request !== requestId.current) return;
      onIdentityChange?.(userData.id);
      setUser(userData);
    } catch (error) {
      if (request !== requestId.current) return;
      if (error instanceof Error && 'status' in error && (error as { status: number }).status !== 401) {
        reportError('AuthContext/currentUser', error);
      }
      onIdentityChange?.(null);
      setUser(null);
    } finally {
      if (request === requestId.current) setLoading(false);
    }
  }, [onIdentityChange]);

  const discardPendingUser = useCallback(() => { requestId.current++; }, []);
  useEffect(() => {
    fetchCurrentUser();
    // Route auth still remounts; an old response must not reset the tab's new session.
    return discardPendingUser;
  }, [fetchCurrentUser, discardPendingUser]);

  const login = () => {
    window.location.href = '/auth/google/login';
  };

  const logout = async () => {
    try {
      await api.post('/auth/logout');
      onIdentityChange?.(null);
      setUser(null);
    } catch {
      // Logout failure is non-critical; user is redirected regardless
    } finally {
      window.location.href = '/login';
    }
  };

  const refetchUser = async () => {
    setLoading(true);
    await fetchCurrentUser();
  };

  return (
    <AuthContext.Provider value={{ user, loading, login, logout, refetchUser }}>
      {children}
    </AuthContext.Provider>
  );
};

// eslint-disable-next-line react-refresh/only-export-components
export const useAuth = (): AuthContextType => {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
};

export default AuthContext;
