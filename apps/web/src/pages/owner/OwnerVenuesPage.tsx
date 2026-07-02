import React, { useEffect, useState, useMemo } from 'react';
import { PageShell } from '../../components/layout/PageShell';
import { useAuth } from '../../contexts/AuthContext';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { fetchOwnerVenues } from '../../lib/api';
import type { Venue } from '../../types/venue';
import { Building2, Plus, MapPin, Info, Search, Filter, Wallet, Settings } from 'lucide-react';
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

  // Client-side filters
  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState('ALL');

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

  const filteredVenues = useMemo(() => {
    return venues.filter(v => {
      const matchSearch = v.name.toLowerCase().includes(searchQuery.toLowerCase()) || 
                          v.city.toLowerCase().includes(searchQuery.toLowerCase()) ||
                          (v.address || '').toLowerCase().includes(searchQuery.toLowerCase());
      const matchStatus = statusFilter === 'ALL' ? true : 
                          statusFilter === 'ACTIVE' ? v.status === 'ACTIVE' : 
                          v.status !== 'ACTIVE';
      return matchSearch && matchStatus;
    });
  }, [venues, searchQuery, statusFilter]);

  const activeVenuesCount = venues.filter(v => v.status === 'ACTIVE').length;

  return (
    <PageShell>
      <div className="pt-24 pb-40 max-w-6xl mx-auto px-6">
        {(intent === 'bookings' || intent === 'upcoming_bookings') && (
          <div className="mb-6 p-4 bg-secondary/10 text-secondary rounded-xl flex items-center gap-3 border border-secondary/20">
            <Info className="w-5 h-5 shrink-0" />
            <p className="text-sm font-bold">
              {intent === 'upcoming_bookings' 
                ? 'Pilih "Lihat Pesanan" pada venue untuk melihat pesanan mendatang.' 
                : 'Pilih "Lihat Pesanan" pada venue yang diinginkan untuk mengelola pesanan masuk.'}
            </p>
          </div>
        )}
        
        {/* Header Operasional */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
          <div>
            <h1 className="text-3xl font-extrabold text-text-main mb-2">Manajemen Venue</h1>
            <p className="text-text-muted">Kelola operasional cabang, lapangan, pesanan, dan performa venue Anda.</p>
          </div>
          <button 
            onClick={() => navigate('/owner/venues/new')}
            className="flex items-center gap-2 bg-primary text-white px-5 py-3 rounded-xl font-bold hover:bg-primary/90 transition-all shadow-sm shrink-0"
          >
            <Plus className="w-5 h-5" />
            Tambah Venue
          </button>
        </div>

        {/* Summary Cards */}
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-8">
          <div className="bg-white p-4 rounded-2xl border border-border-main shadow-sm flex items-center gap-3">
            <div className="w-10 h-10 bg-blue-50 text-blue-600 rounded-xl flex items-center justify-center shrink-0">
              <Building2 className="w-5 h-5" />
            </div>
            <div>
              <p className="text-xs font-bold text-text-muted">Total Venue</p>
              <p className="text-lg font-extrabold text-text-main">{isLoading ? '-' : venues.length}</p>
            </div>
          </div>
          <div className="bg-white p-4 rounded-2xl border border-border-main shadow-sm flex items-center gap-3">
            <div className="w-10 h-10 bg-green-50 text-green-600 rounded-xl flex items-center justify-center shrink-0">
              <Building2 className="w-5 h-5" />
            </div>
            <div>
              <p className="text-xs font-bold text-text-muted">Venue Aktif</p>
              <p className="text-lg font-extrabold text-text-main">{isLoading ? '-' : activeVenuesCount}</p>
            </div>
          </div>
        </div>

        {/* Filters & Search */}
        <div className="bg-white p-3 rounded-2xl border border-border-main shadow-sm mb-8 flex flex-col md:flex-row gap-3">
          <div className="relative flex-1">
            <Search className="w-5 h-5 text-gray-400 absolute left-3 top-1/2 -translate-y-1/2" />
            <input 
              type="text" 
              placeholder="Cari nama atau kota venue..." 
              className="w-full h-10 pl-10 pr-4 rounded-xl bg-gray-50 border-none outline-none text-sm focus:ring-2 focus:ring-primary/20"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
            />
          </div>
          <div className="flex items-center gap-2">
            <Filter className="w-5 h-5 text-gray-400 shrink-0 ml-2 md:ml-0" />
            <select 
              className="h-10 px-4 rounded-xl bg-gray-50 border-none outline-none text-sm focus:ring-2 focus:ring-primary/20 cursor-pointer min-w-[140px]"
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value)}
            >
              <option value="ALL">Semua Status</option>
              <option value="ACTIVE">Aktif</option>
              <option value="INACTIVE">Nonaktif</option>
            </select>
          </div>
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
        ) : filteredVenues.length === 0 ? (
          <div className="bg-white rounded-3xl p-12 border border-border-main text-center shadow-sm">
            <p className="text-text-muted">Tidak ada venue yang cocok dengan pencarian Anda.</p>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            {filteredVenues.map(venue => (
              <div key={venue.id} className="bg-white rounded-2xl border border-border-main p-4 shadow-sm hover:shadow-md transition-shadow flex flex-col">
                <div className="flex gap-4 mb-4">
                  <div className="w-20 h-20 rounded-xl overflow-hidden bg-gray-100 border border-border-main shrink-0">
                    <SafeVenueImage 
                      src={venue.primary_photo}
                      venueId={venue.id}
                      alt={venue.name}
                      className="w-full h-full object-cover"
                      fallbackIcon="building"
                    />
                  </div>
                  <div className="min-w-0 flex-1 flex flex-col justify-center">
                    <div className="flex items-center gap-2 mb-1">
                      <span className={`px-2 py-0.5 rounded-md text-[9px] font-extrabold ${venue.status === 'ACTIVE' ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-600'}`}>
                        {venue.status === 'ACTIVE' ? 'AKTIF' : venue.status || 'NONAKTIF'}
                      </span>
                    </div>
                    <h3 className="font-extrabold text-base text-text-main leading-tight line-clamp-1" title={venue.name}>{venue.name}</h3>
                    <div className="flex items-start gap-1 mt-1 text-text-muted text-xs">
                      <MapPin className="w-3.5 h-3.5 shrink-0 text-primary" />
                      <span className="line-clamp-1">{venue.city}</span>
                    </div>
                  </div>
                </div>

                <div className="mt-auto space-y-2">
                  <button 
                    onClick={() => navigate(`/owner/venues/${venue.id}/courts`)}
                    className="w-full py-2.5 bg-primary text-white font-bold rounded-xl text-sm transition-colors shadow-sm flex justify-center items-center gap-2 hover:bg-primary/90"
                  >
                    <Settings className="w-4 h-4" />
                    Kelola Operasional
                  </button>
                  <div className="grid grid-cols-3 gap-2">
                    <button 
                      onClick={() => navigate(`/owner/venues/${venue.id}/edit`)}
                      className="w-full py-2 bg-gray-50 hover:bg-gray-100 text-text-main font-bold rounded-xl text-xs transition-colors border border-border-main/50"
                    >
                      Edit
                    </button>
                    <button 
                      onClick={() => navigate(`/owner/bookings?venue_id=${venue.id}`)}
                      className="w-full py-2 bg-secondary/10 hover:bg-secondary/20 text-secondary font-bold rounded-xl text-xs transition-colors"
                    >
                      Pesanan
                    </button>
                    <button 
                      onClick={() => navigate(`/owner/finance?venue_id=${venue.id}`)}
                      className="w-full py-2 bg-blue-50 hover:bg-blue-100 text-blue-600 font-bold rounded-xl text-xs transition-colors border border-blue-100 flex items-center justify-center gap-1"
                    >
                      <Wallet className="w-3.5 h-3.5" />
                      Finance
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

