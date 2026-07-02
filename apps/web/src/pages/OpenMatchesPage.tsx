import React, { useState, useEffect, useCallback } from 'react';
import { PageShell } from '../components/layout/PageShell';
import { fetchOpenMatches } from '../lib/api';
import type { OpenMatch } from '../types/mabar';
import { MabarCard } from '../components/MabarCard';
import { LoadingState } from '../components/feedback/LoadingState';
import { ErrorState } from '../components/feedback/ErrorState';
import { EmptyState } from '../components/feedback/EmptyState';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';
import { Pagination } from '../components/ui/Pagination';

export const OpenMatchesPage: React.FC = () => {
  const [matches, setMatches] = useState<OpenMatch[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  
  const navigate = useNavigate();
  const { isAuthenticated } = useAuth();

  const loadMatches = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      const data = await fetchOpenMatches(page, 10);
      setMatches(data.data || []);
      setTotalPages(data.total_pages || 1);
    } catch (err: any) {
      setError(err.message || 'Gagal memuat daftar mabar');
    } finally {
      setIsLoading(false);
    }
  }, [page]);

  useEffect(() => {
    loadMatches();
  }, [loadMatches]);

  return (
    <PageShell>
      <div className="pt-24 pb-40 max-w-7xl mx-auto px-6">
        <div className="mb-10 text-center flex flex-col md:flex-row justify-between items-center gap-6">
          <div className="text-left">
            <h1 className="text-3xl md:text-5xl font-extrabold text-text-main mb-4">
              Cari Lawan / Open Match
            </h1>
            <p className="text-lg text-text-muted max-w-2xl">
              Temukan teman main baru, ikuti mabar, atau buat jadwal mabar Anda sendiri.
            </p>
          </div>
          <button
            onClick={() => navigate(isAuthenticated ? '/bookings' : '/login')}
            className="px-6 py-3.5 rounded-xl font-bold transition-all bg-primary text-white shadow-sm hover:shadow-md hover:-translate-y-1 w-full md:w-auto shrink-0 whitespace-nowrap"
          >
            Buat Jadwal Mabar
          </button>
        </div>

        {isLoading ? (
          <LoadingState message="Mencari jadwal mabar yang tersedia..." />
        ) : error ? (
          <ErrorState message={error} onRetry={loadMatches} />
        ) : matches.length === 0 ? (
          <EmptyState 
            title="Belum ada Mabar" 
            description="Belum ada jadwal open match yang tersedia saat ini." 
          />
        ) : (
          <>
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6">
              {matches.map((match) => (
                <MabarCard key={match.id} match={match} />
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
