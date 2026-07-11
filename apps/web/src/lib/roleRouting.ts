export const getRoleHomeRoute = (role?: string): string => {
  if (role === 'SUPER_ADMIN') {
    return '/admin/dashboard';
  }
  if (role === 'OWNER' || role === 'STAFF') {
    return '/owner/dashboard';
  }
  return '/';
};
