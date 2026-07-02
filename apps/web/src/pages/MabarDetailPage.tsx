import React, { useEffect, useState, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { PageShell } from '../components/layout/PageShell';
import { useAuth } from '../contexts/AuthContext';
import { fetchOpenMatchById, joinOpenMatch, leaveOpenMatch, cancelOpenMatch } from '../lib/api';
import type { OpenMatchDetailResponse } from '../types/mabar';
import { LoadingState } from '../components/feedback/LoadingState';
import { ErrorState } from '../components/feedback/ErrorState';
import { 
  Calendar, MapPin, Users, Trophy, ChevronLeft, 
  Wallet, CheckCircle, AlertCircle, XCircle 
} from 'lucide-react';
import { ConfirmModal } from '../components/ui/ConfirmModal';
import { formatRupiah, formatDate } from '../lib/utils';
import toast from 'react-hot-toast';

export const MabarDetailPage: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { token, user, isAuthenticated } = useAuth();
  
  const [data, setData] = useState<OpenMatchDetailResponse | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  
  // Modals
  const [leaveModalOpen, setLeaveModalOpen] = useState(false);
  const [cancelModalOpen, setCancelModalOpen] = useState(false);

  const loadDetail = useCallback(async () => {
    if (!id) return;
    try {
      setIsLoading(true);
      setError(null);
      const res = await fetchOpenMatchById(id);
      setData(res);
    } catch (err: any) {
      setError(err.message || 'Gagal memuat detail mabar');
    } finally {
      setIsLoading(false);
    }
  }, [id]);

  useEffect(() => {
    loadDetail();
  }, [loadDetail]);

  const handleJoin = async () => {
    if (!isAuthenticated) {
      navigate('/login', { state: { returnTo: `/open-matches/${id}` } });
      return;
    }
    if (!token || !id) return;
    try {
      setActionLoading('join');
      await joinOpenMatch(id, token);
      toast.success('Berhasil bergabung ke mabar!');
      await loadDetail();
    } catch (err: any) {
      toast.error(err.message || 'Gagal bergabung ke mabar');
    } finally {
      setActionLoading(null);
    }
  };

  const confirmLeave = () => setLeaveModalOpen(true);

  const handleLeave = async () => {
    if (!token || !id) return;
    try {
      setActionLoading('leave');
      setLeaveModalOpen(false);
      await leaveOpenMatch(id, token);
      toast.success('Berhasil keluar dari mabar');
      await loadDetail();
    } catch (err: any) {
      toast.error(err.message || 'Gagal keluar dari mabar');
    } finally {
      setActionLoading(null);
    }
  };

  const confirmCancel = () => setCancelModalOpen(true);

  const handleCancel = async () => {
    if (!token || !id) return;
    try {
      setActionLoading('cancel');
      setCancelModalOpen(false);
      await cancelOpenMatch(id, token);
      toast.success('Mabar berhasil dibatalkan');
      await loadDetail();
    } catch (err: any) {
      toast.error(err.message || 'Gagal membatalkan mabar');
    } finally {
      setActionLoading(null);
    }
  };

  if (isLoading) {
    return (
      <PageShell>
        <div className="pt-24 pb-40 max-w-4xl mx-auto px-6">
          <LoadingState message="Memuat detail mabar..." />
        </div>
      </PageShell>
    );
  }

  if (error || !data) {
    return (
      <PageShell>
        <div className="pt-24 pb-40 max-w-4xl mx-auto px-6">
          <ErrorState message={error || 'Data tidak ditemukan'} onRetry={loadDetail} />
        </div>
      </PageShell>
    );
  }

  const match = data.open_match;
  const participants = data.participants || [];
  
  const isHost = user && user.id === match.host_user_id;
  const isParticipant = participants.some(p => p.user_id === user?.id && p.status === 'JOINED');
  const isFull = match.remaining_slots === 0;
  


  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'OPEN':
        return <span className="px-3 py-1 bg-green-100 text-green-800 text-xs font-bold rounded-full flex items-center gap-1"><CheckCircle className="w-3 h-3" /> Tersedia</span>;
      case 'FULL':
        return <span className="px-3 py-1 bg-blue-100 text-blue-800 text-xs font-bold rounded-full flex items-center gap-1"><Users className="w-3 h-3" /> Penuh</span>;
      case 'CANCELLED':
        return <span className="px-3 py-1 bg-red-100 text-red-800 text-xs font-bold rounded-full flex items-center gap-1"><XCircle className="w-3 h-3" /> Dibatalkan</span>;
      default:
        return <span className="px-3 py-1 bg-gray-100 text-gray-800 text-xs font-bold rounded-full">{status}</span>;
    }
  };

  const getLevelColor = (level: string) => {
    switch(level) {
      case 'Beginner / Fun':
      case 'BEGINNER': return 'bg-green-100 text-green-800';
      case 'Intermediate':
      case 'INTERMEDIATE': return 'bg-yellow-100 text-yellow-800';
      case 'Advanced':
      case 'ADVANCED': return 'bg-red-100 text-red-800';
      case 'All Levels':
      case 'ALL_LEVELS': return 'bg-purple-100 text-purple-800';
      default: return 'bg-gray-100 text-gray-800';
    }
	};

  return (
    <PageShell>
      <div className="pt-24 pb-40 max-w-4xl mx-auto px-6">
        <button 
          onClick={() => navigate(-1)}
          className="flex items-center gap-2 text-text-muted hover:text-text-main font-medium mb-8 transition-colors"
        >
          <ChevronLeft className="w-5 h-5" />
          Kembali
        </button>

        <div className="bg-white rounded-3xl overflow-hidden border border-border-main shadow-sm mb-8">
          <div className="p-8 border-b border-border-main">
            <div className="flex flex-wrap gap-2 mb-4">
              {getStatusBadge(match.status)}
              <span className={`px-3 py-1 text-xs font-bold rounded-full flex items-center gap-1 ${getLevelColor(match.level)}`}>
                <Trophy className="w-3 h-3" /> {match.level}
              </span>
              <span className="px-3 py-1 bg-orange-100 text-orange-800 text-xs font-bold rounded-full">
                {match.sport_name}
              </span>
            </div>
            
            <h1 className="text-3xl font-extrabold text-text-main mb-2">{match.title}</h1>
            <p className="text-text-muted flex items-center gap-2 font-medium">
              Dituanrumahi oleh <span className="text-text-main font-bold">{match.host_name}</span>
              {isHost && <span className="text-xs bg-primary/10 text-primary px-2 py-0.5 rounded-full font-bold">Anda</span>}
            </p>
          </div>

          <div className="p-8 bg-gray-50/50 grid grid-cols-1 md:grid-cols-2 gap-8">
            <div className="space-y-6">
              <div>
                <h3 className="text-sm font-bold text-text-muted mb-3 flex items-center gap-2">
                  <Calendar className="w-4 h-4" /> Jadwal Main
                </h3>
                <div className="bg-white p-4 rounded-xl border border-border-main">
                  <p className="font-bold text-text-main mb-1">{formatDate(match.match_date)}</p>
                  <p className="text-primary font-bold">{match.start_time} - {match.end_time}</p>
                </div>
              </div>

              <div>
                <h3 className="text-sm font-bold text-text-muted mb-3 flex items-center gap-2">
                  <MapPin className="w-4 h-4" /> Lokasi Lapangan
                </h3>
                <div className="bg-white p-4 rounded-xl border border-border-main">
                  <p className="font-bold text-text-main mb-1">{match.venue_name}</p>
                  <p className="text-text-muted text-sm font-medium">{match.court_name}</p>
                </div>
              </div>

              <div>
                <h3 className="text-sm font-bold text-text-muted mb-3 flex items-center gap-2">
                  <AlertCircle className="w-4 h-4" /> Catatan Tambahan
                </h3>
                <div className="bg-white p-4 rounded-xl border border-border-main">
                  <p className="text-text-main text-sm leading-relaxed whitespace-pre-wrap">
                    {match.description || 'Tidak ada catatan tambahan dari host.'}
                  </p>
                </div>
              </div>
            </div>

            <div>
              <div className="bg-white p-6 rounded-2xl border border-border-main shadow-sm sticky top-24">
                <div className="flex items-center justify-between mb-6 pb-6 border-b border-border-main">
                  <div>
                    <p className="text-sm font-bold text-text-muted mb-1 flex items-center gap-2">
                      <Wallet className="w-4 h-4" /> Patungan
                    </p>
                    <p className="text-2xl font-extrabold text-primary">
                      {match.price_per_player > 0 ? formatRupiah(match.price_per_player) : 'GRATIS'}
                      <span className="text-sm text-text-muted font-medium">/orang</span>
                    </p>
                  </div>
                </div>

                <div className="mb-6">
                  <div className="flex justify-between text-sm font-bold mb-2">
                    <span className="text-text-main">Slot Terisi</span>
                    <span className="text-primary">{match.joined_count} / {match.max_players}</span>
                  </div>
                  <div className="h-3 w-full bg-gray-100 rounded-full overflow-hidden">
                    <div 
                      className="h-full bg-primary rounded-full transition-all duration-500"
                      style={{ width: `${(match.joined_count / match.max_players) * 100}%` }}
                    />
                  </div>
                  <p className="text-center text-xs font-bold text-text-muted mt-3">
                    {match.remaining_slots > 0 ? `Sisa ${match.remaining_slots} slot lagi!` : 'Slot sudah penuh'}
                  </p>
                </div>

                <div className="space-y-3">
                  {match.status === 'CANCELLED' ? (
                    <button disabled className="w-full py-3.5 rounded-xl bg-gray-100 text-gray-500 font-bold cursor-not-allowed">
                      Mabar Dibatalkan
                    </button>
                  ) : isHost ? (
                    <button 
                      onClick={confirmCancel}
                      disabled={actionLoading !== null}
                      className="w-full py-3.5 rounded-xl border-2 border-red-100 text-red-600 font-bold hover:bg-red-50 transition-colors disabled:opacity-50"
                    >
                      {actionLoading === 'cancel' ? 'Memproses...' : 'Batalkan Mabar (Host)'}
                    </button>
                  ) : isParticipant ? (
                    <button 
                      onClick={confirmLeave}
                      disabled={actionLoading !== null}
                      className="w-full py-3.5 rounded-xl border-2 border-gray-200 text-gray-600 font-bold hover:bg-gray-50 transition-colors disabled:opacity-50"
                    >
                      {actionLoading === 'leave' ? 'Memproses...' : 'Keluar dari Mabar'}
                    </button>
                  ) : (
                    <button 
                      onClick={handleJoin}
                      disabled={isFull || actionLoading !== null}
                      className="w-full py-3.5 rounded-xl bg-primary text-white font-bold hover:bg-primary/90 transition-colors shadow-md disabled:opacity-50 disabled:bg-gray-300 disabled:shadow-none"
                    >
                      {actionLoading === 'join' ? 'Memproses...' : isFull ? 'Slot Penuh' : 'Ikut Patungan Sekarang'}
                    </button>
                  )}
                </div>
              </div>
            </div>
          </div>
        </div>

        <div className="bg-white rounded-3xl p-8 border border-border-main shadow-sm">
          <h2 className="text-xl font-extrabold text-text-main mb-6 flex items-center gap-2">
            <Users className="w-5 h-5 text-primary" /> 
            Daftar Pemain ({participants.filter(p => p.status === 'JOINED').length})
          </h2>
          
          <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-4">
            {participants.filter(p => p.status === 'JOINED').map((p) => (
              <div key={p.id} className="flex items-center gap-3 p-3 rounded-xl border border-border-main bg-gray-50/50">
                <div className="w-10 h-10 rounded-full bg-gradient-to-br from-primary/20 to-primary/10 flex items-center justify-center text-primary font-bold text-sm">
                  {p.name.substring(0, 2).toUpperCase()}
                </div>
                <div>
                  <p className="font-bold text-sm text-text-main leading-tight">{p.name}</p>
                  <p className="text-xs text-text-muted font-medium mt-0.5">
                    {p.user_id === match.host_user_id ? 'Host' : 'Pemain'}
                  </p>
                </div>
              </div>
            ))}
            
            {Array.from({ length: match.remaining_slots }).map((_, i) => (
              <div key={`empty-${i}`} className="flex items-center gap-3 p-3 rounded-xl border border-dashed border-gray-300 bg-gray-50/30">
                <div className="w-10 h-10 rounded-full border-2 border-dashed border-gray-300 flex items-center justify-center text-gray-400">
                  <Users className="w-4 h-4" />
                </div>
                <div>
                  <p className="font-bold text-sm text-gray-400">Slot Kosong</p>
                  <p className="text-xs text-gray-400 font-medium mt-0.5">Menunggu pemain</p>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>

      <ConfirmModal
        isOpen={leaveModalOpen}
        title="Keluar dari Mabar?"
        message="Apakah Anda yakin ingin keluar dari mabar ini?"
        confirmText="Ya, Keluar"
        cancelText="Batal"
        isDestructive={true}
        onConfirm={handleLeave}
        onCancel={() => setLeaveModalOpen(false)}
        isLoading={actionLoading === 'leave'}
      />

      <ConfirmModal
        isOpen={cancelModalOpen}
        title="Batalkan Mabar?"
        message="PERINGATAN: Membatalkan mabar tidak dapat diurungkan. Apakah Anda yakin ingin membatalkan?"
        confirmText="Ya, Batalkan"
        cancelText="Tutup"
        isDestructive={true}
        onConfirm={handleCancel}
        onCancel={() => setCancelModalOpen(false)}
        isLoading={actionLoading === 'cancel'}
      />

    </PageShell>
  );
};
