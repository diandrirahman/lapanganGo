const integerRupiahFormatter = new Intl.NumberFormat('id-ID');

export function formatIntegerRupiah(value: string | null | undefined): string {
  if (value === null || value === undefined || value === '') return 'Belum tersedia';
  try {
    return `Rp ${integerRupiahFormatter.format(BigInt(value))}`;
  } catch {
    return 'Belum tersedia';
  }
}

export function formatCalendarDate(value: string): string {
  const [year, month, day] = value.split('-').map(Number);
  if (!year || !month || !day) return value;
  return new Intl.DateTimeFormat('id-ID', {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    timeZone: 'UTC',
  }).format(new Date(Date.UTC(year, month - 1, day)));
}

export function formatTrendPeriod(start: string, end: string): string {
  if (start === end) return formatCalendarDate(start);
  return `${formatCalendarDate(start)} – ${formatCalendarDate(end)}`;
}

export function parseIntegerRupiah(value: string | null | undefined): bigint | null {
  if (value === null || value === undefined || value === '') return null;
  try {
    return BigInt(value);
  } catch {
    return null;
  }
}

/**
 * Return a bounded percentage for a visual bar without converting the money
 * amount itself to Number. This keeps values above Number.MAX_SAFE_INTEGER
 * visible instead of silently rendering them as zero.
 */
export function chartIntegerPercent(value: string | null | undefined, maxValue: bigint): number {
  const parsed = parseIntegerRupiah(value);
  if (parsed === null || maxValue <= 0n) return 0;
  const absolute = parsed < 0n ? -parsed : parsed;
  if (absolute === 0n) return 0;
  const maxAbsolute = maxValue < 0n ? -maxValue : maxValue;
  if (maxAbsolute === 0n) return 0;
  const percent = Number((absolute * 100n) / maxAbsolute);
  return Math.max(1, Math.min(100, percent));
}
