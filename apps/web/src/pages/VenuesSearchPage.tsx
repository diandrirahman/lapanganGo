import React, { useState, useEffect, useCallback, useMemo } from 'react';
import { useSearchParams } from 'react-router-dom';
import { PageShell } from '../components/layout/PageShell';
import { fetchVenues, fetchSports, fetchFacilities } from '../lib/api';
import type { Venue } from '../types/venue';
import { VenueCard } from '../components/VenueCard';
import { Pagination } from '../components/ui/Pagination';
import { MapPin, X, Search } from 'lucide-react';
import { LoadingState } from '../components/feedback/LoadingState';
import { ErrorState } from '../components/feedback/ErrorState';
import { EmptyState } from '../components/feedback/EmptyState';

// Debounce helper
function useDebounce<T>(value: T, delay: number): T {
  const [debouncedValue, setDebouncedValue] = useState<T>(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebouncedValue(value), delay);
    return () => clearTimeout(timer);
  }, [value, delay]);
  return debouncedValue;
}

export const VenuesSearchPage: React.FC = () => {
  const [venues, setVenues] = useState<Venue[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [totalPages, setTotalPages] = useState(1);
  const [searchParams, setSearchParams] = useSearchParams();

  const page = parseInt(searchParams.get('page') || '1', 10);
  const q = searchParams.get('q') || '';
  const city = searchParams.get('city') || '';
  const minPrice = searchParams.get('minPrice') || '';
  const maxPrice = searchParams.get('maxPrice') || '';
  const sportId = searchParams.get('sportId') || '';
  const facilityIdsStr = searchParams.getAll('facilityId').join(',');
  const facilityIds = useMemo(() => facilityIdsStr ? facilityIdsStr.split(',') : [], [facilityIdsStr]);

  const updateParams = (updates: Record<string, string | string[] | null>) => {
    const newParams = new URLSearchParams(searchParams);
    for (const [key, value] of Object.entries(updates)) {
      if (value === null || value === '' || (Array.isArray(value) && value.length === 0)) {
        newParams.delete(key);
      } else if (Array.isArray(value)) {
        newParams.delete(key);
        value.forEach(v => newParams.append(key, v));
      } else {
        newParams.set(key, value.toString());
      }
    }
    setSearchParams(newParams, { replace: true });
  };

  const setPage = (p: number) => updateParams({ page: p.toString() });
  const setQ = (value: string) => updateParams({ q: value, page: '1' });
  const setCity = (c: string) => updateParams({ city: c, page: '1' });
  const setMinPrice = (p: string) => updateParams({ minPrice: p, page: '1' });
  const setMaxPrice = (p: string) => updateParams({ maxPrice: p, page: '1' });
  const setSportId = (s: string) => updateParams({ sportId: s, page: '1' });
  const setFacilityIds = (updater: (prev: string[]) => string[]) => {
    const next = updater(facilityIds);
    updateParams({ facilityId: next, page: '1' });
  };
  
  const [sports, setSports] = useState<any[]>([]);
  const [facilities, setFacilities] = useState<any[]>([]);

  useEffect(() => {
    fetchSports().then(setSports).catch(console.error);
    fetchFacilities().then(setFacilities).catch(console.error);
  }, []);

  const debouncedQ = useDebounce(q, 500);
  const debouncedCity = useDebounce(city, 500);
  const debouncedMinPrice = useDebounce(minPrice, 500);
  const debouncedMaxPrice = useDebounce(maxPrice, 500);
  const debouncedSportId = useDebounce(sportId, 500);

  const loadVenues = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await fetchVenues(page, 20, {
        q: debouncedQ || undefined,
        city: debouncedCity || undefined,
        sport_id: debouncedSportId || undefined,
        facility_ids: facilityIds.length > 0 ? facilityIds : undefined,
        min_price: debouncedMinPrice ? Number(debouncedMinPrice) : undefined,
        max_price: debouncedMaxPrice ? Number(debouncedMaxPrice) : undefined,
      });
      setVenues(data.data || []);
      setTotalPages(data.total_pages || 1);
    } catch (err: any) {
      setError(err.message || 'Gagal memuat daftar venue');
    } finally {
      setIsLoading(false);
    }
  }, [debouncedQ, debouncedCity, debouncedMinPrice, debouncedMaxPrice, debouncedSportId, facilityIds, page]);



  useEffect(() => {
    loadVenues();
  }, [loadVenues]);

  return (
    <PageShell>
      <div className="pt-24 pb-40 max-w-7xl mx-auto px-6">
        <div className="mb-10 text-center">
          <h1 className="text-3xl md:text-5xl font-extrabold text-text-main mb-4">
            Cari Venue Lapangan
          </h1>
          <p className="text-lg text-text-muted max-w-2xl mx-auto">
            Temukan dan booking lapangan olahraga terbaik di sekitar Anda dengan mudah.
          </p>
        </div>

        {/* Filter Section */}
        <div className="bg-surface rounded-3xl p-6 md:p-8 shadow-sm border border-border-main mb-10">
          <div className="flex items-center justify-between mb-6">
            <h2 className="text-xl font-extrabold text-text-main flex items-center gap-2">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5"><polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3"/></svg>
              Filter Pencarian
            </h2>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-5">
            {/* Search Q Filter */}
            <div className="flex flex-col gap-1.5 md:col-span-2">
              <label className="text-sm font-bold text-text-main">Nama Venue</label>
              <div className="relative">
                <div className="absolute inset-y-0 left-0 pl-4 flex items-center pointer-events-none">
                  <Search className="w-4 h-4 text-text-muted" />
                </div>
                <input
                  type="text"
                  placeholder="Cari nama venue, kota, atau alamat"
                  className="w-full pl-10 pr-10 py-3 rounded-xl border border-border-main focus:ring-2 focus:ring-primary/20 focus:border-primary outline-none transition-all text-sm font-medium bg-bg-main"
                  value={q}
                  onChange={(e) => setQ(e.target.value)}
                />
                {q && (
                  <button 
                    onClick={() => setQ('')}
                    className="absolute inset-y-0 right-0 pr-3 flex items-center text-text-muted hover:text-text-main transition-colors"
                  >
                    <X className="w-4 h-4" />
                  </button>
                )}
              </div>
            </div>

            {/* City Filter */}
            <div className="flex flex-col gap-1.5">
              <label className="text-sm font-bold text-text-main">Kota</label>
              <div className="relative">
                <div className="absolute inset-y-0 left-0 pl-4 flex items-center pointer-events-none">
                  <MapPin className="w-4 h-4 text-text-muted" />
                </div>
                <input
                  type="text"
                  placeholder="Misal: Jakarta"
                  className="w-full pl-10 pr-10 py-3 rounded-xl border border-border-main focus:ring-2 focus:ring-primary/20 focus:border-primary outline-none transition-all text-sm font-medium bg-bg-main"
                  value={city}
                  onChange={(e) => setCity(e.target.value)}
                />
                {city && (
                  <button 
                    onClick={() => setCity('')}
                    className="absolute inset-y-0 right-0 pr-3 flex items-center text-text-muted hover:text-text-main transition-colors"
                  >
                    <X className="w-4 h-4" />
                  </button>
                )}
              </div>
            </div>

            {/* Sport Filter */}
            <div className="flex flex-col gap-1.5">
              <label className="text-sm font-bold text-text-main">Olahraga</label>
              <select
                value={sportId}
                onChange={(e) => setSportId(e.target.value)}
                className="w-full px-4 py-3 rounded-xl border border-border-main focus:ring-2 focus:ring-primary/20 focus:border-primary outline-none transition-all text-sm font-medium bg-bg-main cursor-pointer"
              >
                <option value="">Semua Olahraga</option>
                {sports.map(sport => (
                  <option key={sport.id} value={sport.id}>{sport.name}</option>
                ))}
              </select>
            </div>

            {/* Price Filter */}
            <div className="flex flex-col gap-1.5 md:col-span-2 lg:col-span-2">
              <label className="text-sm font-bold text-text-main">Rentang Harga (Per Jam)</label>
              <div className="flex items-center gap-3">
                <input
                  type="number"
                  placeholder="Min (Rp)"
                  className="w-full px-4 py-3 rounded-xl border border-border-main focus:ring-2 focus:ring-primary/20 focus:border-primary outline-none transition-all text-sm font-medium bg-bg-main"
                  value={minPrice}
                  onChange={(e) => setMinPrice(e.target.value)}
                />
                <span className="text-text-muted font-bold">-</span>
                <input
                  type="number"
                  placeholder="Max (Rp)"
                  className="w-full px-4 py-3 rounded-xl border border-border-main focus:ring-2 focus:ring-primary/20 focus:border-primary outline-none transition-all text-sm font-medium bg-bg-main"
                  value={maxPrice}
                  onChange={(e) => setMaxPrice(e.target.value)}
                />
              </div>
            </div>
          </div>

          {/* Facilities */}
          <div className="mt-6 pt-6 border-t border-border-main">
            <label className="block text-sm font-bold text-text-main mb-3">Fasilitas Tersedia</label>
            <div className="flex flex-wrap gap-2.5">
              {facilities.map(facility => {
                const isSelected = facilityIds.includes(facility.id);
                return (
                  <button
                    key={facility.id}
                    onClick={() => {
                      setFacilityIds(prev => 
                        prev.includes(facility.id) 
                          ? prev.filter(id => id !== facility.id)
                          : [...prev, facility.id]
                      );
                    }}
                    className={`px-4 py-2 rounded-xl text-[13px] font-bold transition-all border flex items-center gap-2 ${
                      isSelected 
                        ? 'bg-primary/10 text-primary border-primary' 
                        : 'bg-bg-main text-text-muted border-transparent hover:border-border-main'
                    }`}
                  >
                    {facility.icon} {facility.name}
                  </button>
                );
              })}
            </div>
          </div>
        </div>

        {/* Results */}
        {isLoading ? (
          <LoadingState message="Mencari venue..." />
        ) : error ? (
          <ErrorState message={error} onRetry={loadVenues} />
        ) : venues.length === 0 ? (
          <EmptyState 
            title="Venue Tidak Ditemukan" 
            description="Tidak ditemukan venue yang cocok dengan kriteria pencarian Anda."
          />
        ) : (
          <>
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6">
              {venues.map((venue) => (
                <VenueCard key={venue.id} venue={venue} />
              ))}
            </div>
            <Pagination
              page={page}
              totalPages={totalPages}
              onPageChange={setPage}
            />
          </>
        )}
      </div>
    </PageShell>
  );
};
