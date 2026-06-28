export interface User {
  id: string;
  name: string;
  email: string;
  phone?: string;
  role: string;
  status: string;
  created_at: string;
}

export interface AuthResponse {
  message: string;
  user: User;
}

export interface LoginResponse {
  message: string;
  token: string;
  user: User;
}
