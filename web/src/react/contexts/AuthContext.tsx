import React, { createContext, useContext, useState, useEffect, useCallback, ReactNode } from 'react';
import { api } from '../../js/api';

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
}

export const AuthProvider: React.FC<AuthProviderProps> = ({ children }) => {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  const fetchCurrentUser = useCallback(async () => {
    try {
      const userData = await api.get<User>('/auth/user');
      setUser(userData);
    } catch (error) {
      if (error instanceof Error && 'status' in error && (error as { status: number }).status !== 401) {
        console.error('Failed to fetch current user', error);
      }
      setUser(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchCurrentUser();
  }, [fetchCurrentUser]);

  const login = () => {
    window.location.href = '/auth/google/login';
  };

  const logout = async () => {
    try {
      await api.post('/auth/logout');
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
