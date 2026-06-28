import React, { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { PageShell } from '../components/layout/PageShell';
import { LoadingState } from '../components/feedback/LoadingState';
import { ErrorState } from '../components/feedback/ErrorState';
import { EmptyState } from '../components/feedback/EmptyState';
import { fetchVenueById } from '../lib/api';
import type { VenueDetail } from '../types/venue';
import { CourtCard } from '../components/CourtCard';
import { SafeVenueImage } from '../components/ui/SafeVenueImage';
import { MapPin, Info } from 'lucide-react';

export const VenueDetailPage: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  
  const [venue, setVenue] = useState<VenueDetail | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!id) return;

    const loadData = async () => {
      try {
        setIsLoading(true);
        setError(null);
        
        const venueData = await fetchVenueById(id);
        
        setVenue(venueData);
      } catch (err: any) {
        setError(err.message || 'Gagal memuat detail venue');
      } finally {
        setIsLoading(false);
      }
    };

    loadData();
  }, [id]);

  if (isLoading) {
    return (
      <PageShell>
        <div className="pt-20 pb-40">
          <LoadingState message="Memuat detail lapangan..." />
        </div>
      </PageShell>
    );
  }

  if (error || !venue) {
    return (
      <PageShell>
        <div className="pt-20 pb-40">
          <ErrorState message={error || 'Venue tidak ditemukan'} onRetry={() => navigate(-1)} />
        </div>
      </PageShell>
    );
  }

  return (
    <PageShell>
      {/* Banner */}
      <div className="w-full max-w-7xl mx-auto px-6 mb-12">
        <div className="w-full h-[300px] md:h-[400px] rounded-3xl overflow-hidden relative shadow-md bg-gray-100 flex items-center justify-center">
          <SafeVenueImage 
            src={venue.primary_photo}
            venueId={venue.id}
            alt={venue.name}
            className="w-full h-full object-cover"
            fallbackIcon="image"
          />
          <div className="absolute inset-0 bg-gradient-to-t from-black/80 via-black/30 to-transparent flex items-end p-8 md:p-12">
            <div className="text-white">
              <div className="flex gap-2 mb-3 flex-wrap">
                {venue.facilities.map(f => (
                  <span key={f.id} className="bg-white/20 backdrop-blur-md px-3 py-1 rounded-full text-[12px] font-bold">
                    {f.name}
                  </span>
                ))}
              </div>
              <h1 className="text-3xl md:text-5xl font-extrabold mb-4 tracking-tight">{venue.name}</h1>
              <div className="flex items-center gap-2 text-white/80 font-medium text-[15px]">
                <MapPin className="w-5 h-5" />
                <span>{venue.address}, {venue.city}</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Content */}
      <div className="max-w-7xl mx-auto px-6 grid grid-cols-1 lg:grid-cols-3 gap-12">
        {/* Left Column - Details */}
        <div className="lg:col-span-1 space-y-6">
          <div className="bg-surface rounded-3xl p-6 md:p-8 shadow-sm border border-border-main">
            <h3 className="text-[20px] font-extrabold text-text-main mb-4 flex items-center gap-2">
              <Info className="w-5 h-5 text-primary" />
              Tentang Venue
            </h3>
            <p className="text-text-muted leading-relaxed font-medium">
              {venue.description || 'Tidak ada deskripsi tersedia untuk venue ini. Lapangan ini menawarkan fasilitas premium dengan lokasi yang strategis untuk kegiatan olahraga Anda.'}
            </p>
          </div>
          
          {venue.photos && venue.photos.length > 0 && (
            <div className="bg-surface rounded-3xl p-6 md:p-8 shadow-sm border border-border-main">
              <h3 className="text-[20px] font-extrabold text-text-main mb-4">Galeri Foto</h3>
              <div className="grid grid-cols-2 gap-3">
                {venue.photos.map(photo => {
                  const GalleryImage = () => {
                    return (
                      <div key={photo.id} className="rounded-xl overflow-hidden aspect-square bg-gray-100 flex items-center justify-center group">
                        <SafeVenueImage 
                          src={photo.image_url}
                          venueId={venue.id}
                          alt={photo.alt_text || 'Foto Venue'}
                          className="w-full h-full object-cover group-hover:scale-110 transition-transform duration-300"
                          fallbackIcon="image"
                        />
                      </div>
                    );
                  };
                  return <GalleryImage key={photo.id} />;
                })}
              </div>
            </div>
          )}
        </div>

        {/* Right Column - Courts */}
        <div className="lg:col-span-2">
          <h3 className="text-[24px] font-extrabold text-text-main mb-6">Pilih Lapangan</h3>
          
          {venue.courts.length === 0 ? (
            <EmptyState 
              title="Belum Ada Lapangan" 
              description="Venue ini belum mendaftarkan lapangan yang bisa disewa."
            />
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
              {venue.courts.map(court => (
                <CourtCard 
                  key={court.id} 
                  court={court} 
                  onSelect={(courtId) => navigate(`/venues/${venue.id}/courts/${courtId}/availability`, { 
                    state: { 
                      venue: { name: venue.name, address: venue.address }, 
                      court: { name: court.name, price_per_hour: court.price_per_hour } 
                    } 
                  })}
                />
              ))}
            </div>
          )}
        </div>
      </div>
    </PageShell>
  );
};
