import React, { useEffect, useState } from 'react';
import { ArrowRight } from 'lucide-react';
import { Link } from 'react-router-dom';
import type { Venue } from '../types/venue';
import { fetchVenues } from '../lib/api';
import { VenueCard } from './VenueCard';
import { EmptyState } from './feedback/EmptyState';
import { ErrorState } from './feedback/ErrorState';
import { LoadingState } from './feedback/LoadingState';
import { ScrollReveal } from './ui/ScrollReveal';

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
    <section id="venue-pilihan" className="relative z-20 bg-white py-16 md:py-28">
      <div className="mx-auto max-w-7xl px-5 md:px-6">

        {/* Header */}
        <ScrollReveal className="mb-10 flex flex-col items-start justify-between gap-6 md:mb-14 md:flex-row md:items-end">
          <div>
            <p className="mb-4 text-xs font-extrabold uppercase tracking-[0.2em] text-primary">Rekomendasi Venue</p>
            <h2 className="max-w-4xl text-[clamp(2.6rem,6vw,5.4rem)] font-extrabold leading-[0.95] tracking-[-0.055em] text-text-main">Tempat terbaik untuk mulai bergerak.</h2>
            <p className="mt-5 text-base font-medium text-text-muted md:text-lg">
              Lapangan terbaik yang bisa langsung dibooking hari ini.
            </p>
          </div>
          <Link to="/venues" className="group flex shrink-0 items-center gap-3 rounded-full border border-border-main bg-white px-5 py-3 font-bold text-text-main transition-all hover:-translate-y-0.5 hover:border-primary/30 hover:text-primary hover:shadow-md active:scale-95">
            Lihat Semua <ArrowRight className="h-4 w-4 transition-transform group-hover:translate-x-1" />
          </Link>
        </ScrollReveal>

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
          <div className="grid grid-cols-1 gap-5 sm:grid-cols-2 xl:grid-cols-4">
            {venues.map((venue, index) => (
              <ScrollReveal key={venue.id} delay={index * 70} className="h-full">
                <VenueCard venue={venue} />
              </ScrollReveal>
            ))}
          </div>
        )}
      </div>
    </section>
  );
};
