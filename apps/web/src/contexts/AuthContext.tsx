import React, { createContext, useContext, useState, useEffect } from 'react';
import type { ReactNode } from 'react';
import type { User } from '../types/auth';

interface AuthContextType {
  user: User | null;
  token: string | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  login: (token: string, user: User) => void;
  logout: () => void;
  isActualOwner: () => boolean;
  hasOwnerPermission: (permission: string) => boolean;
  isWorkspaceUser: () => boolean;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080';

export const AuthProvider: React.FC<{ children: ReactNode }> = ({ children }) => {
  const [user, setUser] = useState<User | null>(null);
  const [token, setToken] = useState<string | null>(localStorage.getItem('auth_token'));
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    const fetchMe = async () => {
      if (!token) {
        setIsLoading(false);
        return;
      }

      try {
        // If we want to support mock mode for auth as well:
        if (import.meta.env.VITE_USE_MOCK_AUTH === 'true') {
          setUser({
            id: 'mock-user-1',
            name: 'QA Tester',
            email: 'qa@lapanggo.id',
            role: 'CUSTOMER',
            status: 'ACTIVE',
            created_at: new Date().toISOString()
          });
          setIsLoading(false);
          return;
        }

        const response = await fetch(`${API_BASE_URL}/auth/me`, {
          headers: {
            'Authorization': `Bearer ${token}`
          }
        });

        if (response.ok) {
          const data = await response.json();
          setUser(data.user);
        } else {
          // Token invalid or expired
          logout();
        }
      } catch (error) {
        console.error('Failed to fetch user:', error);
      } finally {
        setIsLoading(false);
      }
    };

    fetchMe();
  }, [token]);

  const login = (newToken: string, newUser: User) => {
    localStorage.setItem('auth_token', newToken);
    setToken(newToken);
    setUser(newUser);
  };

  const logout = () => {
    localStorage.removeItem('auth_token');
    setToken(null);
    setUser(null);
  };

  const isActualOwner = (): boolean => {
    return (user?.role === 'OWNER' && !!user?.owner_profile) || false;
  };

  const isWorkspaceUser = (): boolean => {
    return isActualOwner() || ((user?.staff_memberships?.length ?? 0) > 0);
  };

  const hasOwnerPermission = (permission: string) => {
    if (!user) return false;
    if (isActualOwner()) return true; // Actual owner has all permissions
    if (user.staff_memberships && user.staff_memberships.length > 0) {
      return user.staff_memberships[0].permissions.includes(permission);
    }
    return false;
  };

  return (
    <AuthContext.Provider value={{
      user, token, isAuthenticated: !!user, isLoading,
      login, logout, isActualOwner, hasOwnerPermission, isWorkspaceUser
    }}>
      {children}
    </AuthContext.Provider>
  );
};

// eslint-disable-next-line react-refresh/only-export-components
export const useAuth = () => {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
};
