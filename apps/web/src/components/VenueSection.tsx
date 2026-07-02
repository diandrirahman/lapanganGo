import React, { useEffect, useState } from 'react';
import { ArrowRight } from 'lucide-react';
import { Link } from 'react-router-dom';
import type { Venue } from '../types/venue';
import { fetchVenues } from '../lib/api';
import { VenueCard } from './VenueCard';
import { EmptyState } from './feedback/EmptyState';
import { ErrorState } from './feedback/ErrorState';
import { LoadingState } from './feedback/LoadingState';

export const VenueSection: React.FC = () => {
  const [venues, setVenues] = useState<Venue[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const loadVenues = async () => {
      try {
        setIsLoading(true);
        setError(null);
        const data = await fetchVenues(1, 4);
        setVenues(data.data || []);
      } catch (err: any) {
        setError(err.message || 'Gagal memuat daftar venue');
      } finally {
        setIsLoading(false);
      }
    };

    loadVenues();
  }, []);

  return (
    <section className="py-14 md:py-20 relative z-20 bg-bg-main">
      <div className="max-w-7xl mx-auto px-5 md:px-6">
        
        {/* Header */}
        <div className="flex flex-col md:flex-row justify-between items-start md:items-end mb-10 gap-4">
          <div>
            <h2 className="text-3xl md:text-4xl font-extrabold tracking-tight text-text-main mb-2">Rekomendasi Venue</h2>
            <p className="text-base md:text-lg text-text-muted font-medium">
              Lapangan terbaik yang bisa langsung dibooking hari ini.
            </p>
          </div>
          <Link to="/venues" className="flex items-center gap-2 font-bold text-primary transition-colors shrink-0 bg-primary/10 px-5 py-2.5 rounded-full hover:bg-primary/20 active:scale-95">
            Lihat Semua <ArrowRight className="w-4 h-4" />
          </Link>
        </div>

        {/* Content */}
        {isLoading ? (
          <LoadingState message="Memuat daftar venue..." variant="cards" />
        ) : error ? (
          <ErrorState message={error} onRetry={() => window.location.reload()} />
        ) : venues.length === 0 ? (
          <EmptyState 
            title="Belum Ada Venue" 
            description="Saat ini belum ada venue lapangan yang tersedia di sistem kami."
          />
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8">
            {venues.map((venue) => (
              <VenueCard key={venue.id} venue={venue} />
            ))}
          </div>
        )}
      </div>
    </section>
  );
};
