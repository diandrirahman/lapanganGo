import React, { useEffect, useState } from 'react';
import { PageShell } from '../../components/layout/PageShell';
import { useAuth } from '../../contexts/AuthContext';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { fetchOwnerVenues } from '../../lib/api';
import type { Venue } from '../../types/venue';
import { Building2, Plus, MapPin, Info } from 'lucide-react';
import { LoadingState } from '../../components/feedback/LoadingState';
import { ErrorState } from '../../components/feedback/ErrorState';
import { SafeVenueImage } from '../../components/ui/SafeVenueImage';

export const OwnerVenuesPage: React.FC = () => {
  const { token } = useAuth();
  const [venues, setVenues] = useState<Venue[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const intent = searchParams.get('intent');

  useEffect(() => {
    if (token) {
      fetchOwnerVenues(token)
        .then(setVenues)
        .catch((err) => setError(err.message))
        .finally(() => setIsLoading(false));
    } else {
      setIsLoading(false);
    }
  }, [token]);

  return (
    <PageShell>
      <div className="pt-24 pb-40 max-w-6xl mx-auto px-6">
        {(intent === 'bookings' || intent === 'upcoming_bookings') && (
          <div className="mb-6 p-4 bg-secondary/10 text-secondary-hover rounded-xl flex items-center gap-3 border border-secondary/20">
            <Info className="w-5 h-5 shrink-0" />
            <p className="text-sm font-bold">
              {intent === 'upcoming_bookings' 
                ? 'Pilih "Lihat Pesanan" pada venue untuk melihat pesanan mendatang.' 
                : 'Pilih "Lihat Pesanan" pada venue yang diinginkan untuk mengelola pesanan masuk.'}
            </p>
          </div>
        )}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
          <div>
            <h1 className="text-3xl font-extrabold text-text-main mb-2">Venue Saya</h1>
            <p className="text-text-muted">Kelola daftar venue dan lapangan yang Anda miliki.</p>
          </div>
          <button 
            onClick={() => navigate('/owner/venues/new')}
            className="flex items-center gap-2 bg-primary text-white px-5 py-3 rounded-xl font-bold hover:bg-primary/90 transition-all shadow-sm"
          >
            <Plus className="w-5 h-5" />
            Tambah Venue
          </button>
        </div>

        {isLoading ? (
          <LoadingState message="Memuat venue Anda..." className="bg-white rounded-3xl" />
        ) : error ? (
          <ErrorState message={error} onRetry={() => window.location.reload()} />
        ) : venues.length === 0 ? (
          <div className="bg-white rounded-3xl p-12 border border-border-main text-center shadow-sm">
            <div className="w-16 h-16 bg-gray-100 text-gray-400 flex items-center justify-center rounded-full mx-auto mb-4">
              <Building2 className="w-8 h-8" />
            </div>
            <h2 className="text-xl font-extrabold text-text-main mb-2">Belum Ada Venue</h2>
            <p className="text-text-muted mb-6">Mulai bisnis Anda dengan menambahkan venue pertama.</p>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            {venues.map(venue => (
              <div key={venue.id} className="bg-white rounded-2xl border border-border-main p-5 shadow-sm hover:shadow-md transition-shadow flex flex-col">
                <div className="relative w-full h-40 rounded-xl mb-4 overflow-hidden bg-gray-100 border border-border-main">
                  <SafeVenueImage 
                    src={venue.primary_photo}
                    venueId={venue.id}
                    alt={venue.name}
                    className="w-full h-full object-cover"
                    fallbackIcon="building"
                  />
                  {venue.status && (
                    <div className="absolute top-2 right-2">
                      <span className={`px-2 py-1 rounded-md text-[10px] font-extrabold shadow-sm ${venue.status === 'ACTIVE' ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-600'}`}>
                        {venue.status === 'ACTIVE' ? 'AKTIF' : venue.status}
                      </span>
                    </div>
                  )}
                </div>
                
                <h3 className="font-extrabold text-lg text-text-main mb-1 line-clamp-1" title={venue.name}>{venue.name}</h3>
                <div className="flex items-start gap-1.5 text-text-muted text-sm mb-3">
                  <MapPin className="w-4 h-4 shrink-0 mt-0.5 text-primary" />
                  <span className="line-clamp-2">{venue.address}, {venue.city}</span>
                </div>
                
                {venue.facilities && venue.facilities.length > 0 && (
                  <div className="flex gap-1.5 flex-wrap mb-4">
                    {venue.facilities.slice(0, 3).map(f => (
                      <span key={f.id} className="bg-gray-100 text-gray-600 px-2 py-0.5 rounded-md text-[11px] font-bold">
                        {f.name}
                      </span>
                    ))}
                    {venue.facilities.length > 3 && (
                      <span className="bg-gray-100 text-gray-600 px-2 py-0.5 rounded-md text-[11px] font-bold">
                        +{venue.facilities.length - 3}
                      </span>
                    )}
                  </div>
                )}

                <div className="mt-auto pt-4 border-t border-border-main/50 space-y-2">
                  <button 
                    onClick={() => navigate(`/owner/venues/${venue.id}/edit`)}
                    className="w-full py-2 bg-blue-50 hover:bg-blue-100 text-blue-600 font-bold rounded-xl text-sm transition-colors border border-blue-100"
                  >
                    Edit Detail & Foto
                  </button>
                  <div className="grid grid-cols-2 gap-2">
                    <button 
                      onClick={() => navigate(`/owner/venues/${venue.id}/courts`)}
                      className="w-full py-2 bg-gray-100 hover:bg-gray-200 text-text-main font-bold rounded-xl text-sm transition-colors"
                    >
                      Kelola Court
                    </button>
                    <button 
                      onClick={() => navigate(`/owner/venues/${venue.id}/bookings${intent === 'upcoming_bookings' ? '?scope=upcoming' : ''}`)}
                      className="w-full py-2 bg-secondary/10 hover:bg-secondary/20 text-secondary font-bold rounded-xl text-sm transition-colors"
                    >
                      Lihat Pesanan
                    </button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </PageShell>
  );
};
