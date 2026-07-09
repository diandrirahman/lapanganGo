import React, { useEffect, useState } from 'react';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import { PageShell } from '../components/layout/PageShell';
import { LoadingState } from '../components/feedback/LoadingState';
import { ErrorState } from '../components/feedback/ErrorState';
import { EmptyState } from '../components/feedback/EmptyState';
import { fetchVenueById } from '../lib/api';
import type { VenueDetail } from '../types/venue';
import { CourtCard } from '../components/CourtCard';
import { SafeVenueImage } from '../components/ui/SafeVenueImage';
import { MapPin, Info, Ticket } from 'lucide-react';
import { useAuth } from '../contexts/AuthContext';
import { toast } from 'react-hot-toast';

export const VenueDetailPage: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { user } = useAuth();
  
  const [venue, setVenue] = useState<VenueDetail | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedPromoCode, setSelectedPromoCode] = useState<string | null>(null);

  useEffect(() => {
    if (!id) return;

    const loadData = async () => {
      try {
        setIsLoading(true);
        setError(null);
        
        const playDate = searchParams.get('play_date') || undefined;
        const venueData = await fetchVenueById(id, playDate);
        
        setVenue(venueData);
      } catch (err: any) {
        setError(err.message || 'Gagal memuat detail venue');
      } finally {
        setIsLoading(false);
      }
    };

    loadData();
  }, [id, searchParams]);

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
                {venue.photos.map((photo, index) => {
                  const imageUrl = typeof photo === 'string' ? photo : (photo as any).image_url;
                  const photoId = typeof photo === 'string' ? `photo-${index}` : (photo as any).id;
                  const altText = typeof photo === 'string' ? 'Foto Venue' : (photo as any).alt_text || 'Foto Venue';
                  
                  const GalleryImage = () => {
                    return (
                      <div key={photoId} className="rounded-xl overflow-hidden aspect-square bg-gray-100 flex items-center justify-center group">
                        <SafeVenueImage 
                          src={imageUrl}
                          venueId={venue.id}
                          alt={altText}
                          className="w-full h-full object-cover group-hover:scale-110 transition-transform duration-300"
                          fallbackIcon="image"
                        />
                      </div>
                    );
                  };
                  return <GalleryImage key={photoId} />;
                })}
              </div>
            </div>
          )}

          {/* Promos */}
          {venue.promos && venue.promos.length > 0 && (
            <div className="bg-surface rounded-3xl p-6 md:p-8 shadow-sm border border-emerald-200">
              <h3 className="text-[20px] font-extrabold text-text-main mb-4 flex items-center gap-2">
                <Ticket className="w-5 h-5 text-emerald-500" />
                Promo Tersedia
              </h3>
              <div className="flex flex-col gap-4">
                {venue.promos.filter(promo => {
                  if (!searchParams.get('play_date')) return true;
                  const [y, m, d] = searchParams.get('play_date')!.split('-').map(Number);
                  const pdDate = new Date(y, m - 1, d);
                  const startsAt = new Date(promo.starts_at);
                  const endsAt = new Date(promo.ends_at);
                  const startDate = new Date(startsAt.getFullYear(), startsAt.getMonth(), startsAt.getDate());
                  const endDate = new Date(endsAt.getFullYear(), endsAt.getMonth(), endsAt.getDate());
                  return pdDate >= startDate && pdDate <= endDate;
                }).map(promo => {
                  const startsAt = new Date(promo.starts_at);
                  const endsAt = new Date(promo.ends_at);
                  const playDateParam = searchParams.get('play_date');
                  
                  let isFuture = false;
                  if (!playDateParam) {
                    isFuture = startsAt > new Date();
                  }
                  
                  return (
                    <div key={promo.id} className="bg-emerald-50/50 rounded-2xl p-4 border border-emerald-100 relative overflow-hidden">
                      <div className="absolute top-0 right-0 p-3 opacity-10">
                        <svg className="w-16 h-16" fill="currentColor" viewBox="0 0 24 24"><path d="M12 2L15.09 8.26L22 9.27L17 14.14L18.18 21.02L12 17.77L5.82 21.02L7 14.14L2 9.27L8.91 8.26L12 2Z"/></svg>
                      </div>
                      
                      <div className="relative z-10">
                        <div className="flex justify-between items-start mb-2">
                          <h4 className="font-extrabold text-emerald-800 text-lg">
                            {promo.code}
                          </h4>
                          <span className="bg-emerald-100 text-emerald-700 text-[10px] font-bold px-2 py-0.5 rounded-full uppercase tracking-wider">
                            {isFuture ? 'Akan Datang' : 'Aktif'}
                          </span>
                        </div>
                        
                        <div className="text-emerald-900 font-bold mb-3">
                          Diskon {promo.discount_type === 'PERCENTAGE' ? `${promo.discount_value}%` : `Rp ${promo.discount_value.toLocaleString('id-ID')}`}
                        </div>
                        
                        <div className="text-sm font-medium text-emerald-700 mb-4">
                          Berlaku untuk jadwal main {startsAt.toLocaleDateString('id-ID', { day: 'numeric', month: 'short', year: 'numeric' })} - {endsAt.toLocaleDateString('id-ID', { day: 'numeric', month: 'short', year: 'numeric' })}
                        </div>
                        
                        <div className="flex gap-2">
                          <button 
                            onClick={() => {
                              setSelectedPromoCode(promo.code);
                              toast.success('Promo dipilih! Silakan pilih lapangan di bawah.');
                            }}
                            className={`text-[13px] font-bold px-4 py-2 rounded-xl transition-colors flex items-center gap-1.5 ${
                              selectedPromoCode === promo.code 
                                ? 'bg-emerald-600 text-white shadow-md' 
                                : 'text-emerald-600 bg-white border border-emerald-200 hover:bg-emerald-50'
                            }`}
                          >
                            <Ticket className="w-4 h-4" />
                            {selectedPromoCode === promo.code ? 'Promo Terpilih' : 'Gunakan Promo'}
                          </button>
                        </div>
                      </div>
                    </div>
                  );
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
                  onSelect={(courtId) => {
                    if (user?.role === 'OWNER' || user?.role === 'STAFF') {
                      toast.error('Gunakan akun customer untuk membuat booking.');
                      return;
                    }
                    const playDateParam = searchParams.get('play_date');
                    let queryStr = playDateParam ? `?play_date=${playDateParam}` : '';
                    if (selectedPromoCode) {
                      queryStr += queryStr ? `&promo=${selectedPromoCode}` : `?promo=${selectedPromoCode}`;
                    }
                    navigate(`/venues/${venue.id}/courts/${courtId}/availability${queryStr}`, { 
                      state: { 
                        venue: { name: venue.name, address: venue.address }, 
                        court: { name: court.name, price_per_hour: court.price_per_hour } 
                      } 
                    });
                  }}
                />
              ))}
            </div>
          )}
        </div>
      </div>
    </PageShell>
  );
};
