function parseStrictBool(value: string | undefined, flagName: string): boolean {
    if (value === undefined || value === '' || value === 'false') {
        return false;
    }
    if (value === 'true') {
        return true;
    }
    throw new Error(`Invalid boolean configuration for ${flagName}: must be exact lowercase 'true' or 'false'`);
}

export function parsePlatformFinanceAdminFlag(value: string | undefined): boolean {
    return value === 'true';
}

export const isPlatformFinanceAdminEnabled = parsePlatformFinanceAdminFlag(
    import.meta.env.VITE_PLATFORM_FINANCE_ADMIN_ENABLED
);

export const isPlatformMonetizationEnabled = parseStrictBool(
    import.meta.env.VITE_PLATFORM_MONETIZATION_ENABLED,
    'VITE_PLATFORM_MONETIZATION_ENABLED'
);

if (isPlatformMonetizationEnabled) {
    throw new Error("VITE_PLATFORM_MONETIZATION_ENABLED=true is strictly prohibited during Phase 4 across all environments");
}
