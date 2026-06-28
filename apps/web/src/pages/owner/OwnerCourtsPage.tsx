import React, { useEffect, useState, useCallback } from 'react';
import { PageShell } from '../../components/layout/PageShell';
import { useAuth } from '../../contexts/AuthContext';
import { useParams } from 'react-router-dom';
import { fetchOwnerVenueById, fetchOwnerCourtsByVenueId } from '../../lib/api';
import type { OwnerVenueDetail } from '../../types/venue';
import { Plus, MapPin } from 'lucide-react';
import { LoadingState } from '../../components/feedback/LoadingState';
import { ErrorState } from '../../components/feedback/ErrorState';
import { CourtModal } from '../../components/owner/CourtModal';
import { OperatingHoursModal } from '../../components/owner/OperatingHoursModal';
import { BlockedSlotsModal } from '../../components/owner/BlockedSlotsModal';

export const OwnerCourtsPage: React.FC = () => {
  const { token } = useAuth();
  const { id: venueId } = useParams<{ id: string }>();
  const [venue, setVenue] = useState<OwnerVenueDetail | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Modal States
  const [isCourtModalOpen, setIsCourtModalOpen] = useState(false);
  const [selectedCourt, setSelectedCourt] = useState<any | null>(null);
  
  const [isHoursModalOpen, setIsHoursModalOpen] = useState(false);
  const [selectedCourtForHours, setSelectedCourtForHours] = useState<{id: string, name: string} | null>(null);
  
  const [isBlockedModalOpen, setIsBlockedModalOpen] = useState(false);
  const [selectedCourtForBlocked, setSelectedCourtForBlocked] = useState<{id: string, name: string} | null>(null);

  const loadVenueData = useCallback(async () => {
    if (!venueId || !token) {
      setIsLoading(false);
      return;
    }
    
    try {
      setIsLoading(true);
      const [venueData, courtsData] = await Promise.all([
        fetchOwnerVenueById(venueId, token),
        fetchOwnerCourtsByVenueId(venueId, token)
      ]);
      
      setVenue({
        ...(venueData as OwnerVenueDetail),
        courts: courtsData
      });
    } catch (err: any) {
      setError(err.message);
    } finally {
      setIsLoading(false);
    }
  }, [venueId, token]);

  useEffect(() => {
    loadVenueData();
  }, [loadVenueData]);

  if (isLoading) {
    return <PageShell><div className="pt-32 text-center text-text-muted">Memuat...</div></PageShell>;
  }

  const formatPrice = (price: number) => {
    return new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 }).format(price);
  };

  return (
    <PageShell>
      <div className="pt-24 pb-40 max-w-6xl mx-auto px-6">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
          <div>
            <h1 className="text-3xl font-extrabold text-text-main mb-2">Kelola Lapangan</h1>
            {venue && (
              <p className="text-text-muted flex items-center gap-1.5 font-medium">
                <MapPin className="w-4 h-4 text-primary" /> {venue.name}
              </p>
            )}
          </div>
          <button 
            onClick={() => { setSelectedCourt(null); setIsCourtModalOpen(true); }}
            className="bg-primary text-white px-6 py-3 rounded-xl font-bold flex items-center justify-center gap-2 hover:bg-primary/90 transition-all shadow-sm"
          >
            <Plus className="w-5 h-5" />
            Tambah Lapangan
          </button>
        </div>

        {isLoading ? (
          <LoadingState message="Memuat daftar lapangan..." className="bg-white rounded-3xl" />
        ) : error ? (
          <ErrorState message={error} onRetry={() => window.location.reload()} />
        ) : !venue?.courts || venue.courts.length === 0 ? (
          <div className="bg-white rounded-3xl p-12 border border-border-main text-center shadow-sm">
            <h2 className="text-xl font-extrabold text-text-main mb-2">Belum Ada Lapangan</h2>
            <p className="text-text-muted mb-6">Tambahkan lapangan baru untuk mulai menerima pesanan.</p>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {venue.courts.map(court => (
              <div key={court.id} className="bg-white rounded-2xl border border-border-main p-5 shadow-sm flex flex-col hover:shadow-md transition-shadow">
                <div className="flex justify-between items-start mb-1">
                  <h3 className="font-extrabold text-lg text-text-main leading-tight">{court.name}</h3>
                  <span className="bg-primary/10 text-primary text-[10px] px-2 py-1 rounded-md font-extrabold shrink-0">
                    {court.sport.name}
                  </span>
                </div>
                
                <div className="bg-gray-50 rounded-xl p-3 my-4 space-y-2 border border-gray-100">
                  <div className="flex justify-between text-xs">
                    <span className="text-text-muted font-medium">Tipe Lokasi</span>
                    <span className="font-bold text-text-main">{court.location_type}</span>
                  </div>
                  <div className="flex justify-between text-xs">
                    <span className="text-text-muted font-medium">Permukaan</span>
                    <span className="font-bold text-text-main">{court.surface_type}</span>
                  </div>
                  <div className="flex justify-between text-xs pt-2 border-t border-gray-200">
                    <span className="text-text-muted font-medium">Harga/Jam</span>
                    <span className="font-extrabold text-primary">{formatPrice(court.price_per_hour)}</span>
                  </div>
                </div>

                <div className="mt-auto grid grid-cols-2 gap-2">
                  <button 
                    onClick={() => { setSelectedCourtForHours({ id: court.id, name: court.name }); setIsHoursModalOpen(true); }}
                    className="w-full py-2 bg-blue-50 text-blue-700 font-bold rounded-xl text-xs hover:bg-blue-100 transition-colors border border-blue-100"
                  >
                    Jam Operasional
                  </button>
                  <button 
                    onClick={() => { setSelectedCourtForBlocked({ id: court.id, name: court.name }); setIsBlockedModalOpen(true); }}
                    className="w-full py-2 bg-red-50 text-red-700 font-bold rounded-xl text-xs hover:bg-red-100 transition-colors border border-red-100"
                  >
                    Blokir Slot
                  </button>
                  <button 
                    onClick={() => { setSelectedCourt(court); setIsCourtModalOpen(true); }}
                    className="col-span-2 w-full py-2 bg-gray-100 text-text-main font-bold rounded-xl text-xs hover:bg-gray-200 transition-colors border border-gray-200"
                  >
                    Edit Detail Lapangan
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {isCourtModalOpen && token && venueId && (
        <CourtModal
          isOpen={isCourtModalOpen}
          onClose={() => setIsCourtModalOpen(false)}
          onSuccess={loadVenueData}
          token={token}
          venueId={venueId}
          court={selectedCourt}
        />
      )}

      {isHoursModalOpen && token && selectedCourtForHours && (
        <OperatingHoursModal
          isOpen={isHoursModalOpen}
          onClose={() => setIsHoursModalOpen(false)}
          token={token}
          courtId={selectedCourtForHours.id}
          courtName={selectedCourtForHours.name}
        />
      )}

      {isBlockedModalOpen && token && selectedCourtForBlocked && (
        <BlockedSlotsModal
          isOpen={isBlockedModalOpen}
          onClose={() => setIsBlockedModalOpen(false)}
          token={token}
          courtId={selectedCourtForBlocked.id}
          courtName={selectedCourtForBlocked.name}
        />
      )}
    </PageShell>
  );
};
