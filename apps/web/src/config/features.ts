export function parsePlatformFinanceAdminFlag(value: string | undefined): boolean {
    return value === 'true';
}

export const isPlatformFinanceAdminEnabled = parsePlatformFinanceAdminFlag(
    import.meta.env.VITE_PLATFORM_FINANCE_ADMIN_ENABLED
);
