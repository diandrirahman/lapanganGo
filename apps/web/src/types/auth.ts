export interface OwnerProfile {
  id: string;
  name: string;
}

export interface StaffMembership {
  id: string;
  owner_profile_id: string;
  owner_name: string;
  role: string;
  permissions: string[];
}

export interface User {
  id: string;
  name: string;
  email: string;
  phone?: string;
  role: string;
  status: string;
  created_at: string;
  owner_profile?: OwnerProfile;
  staff_memberships?: StaffMembership[];
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
