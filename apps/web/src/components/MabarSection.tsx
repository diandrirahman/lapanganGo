import React, { useEffect, useState } from 'react';
import type { OpenMatch } from '../types/mabar';
import { fetchOpenMatches } from '../lib/api';
import { MabarCard } from './MabarCard';
import { Button } from './ui/Button';
import { EmptyState } from './feedback/EmptyState';
import { ErrorState } from './feedback/ErrorState';
import { LoadingState } from './feedback/LoadingState';
import { useNavigate } from 'react-router-dom';

export const MabarSection: React.FC = () => {
  const [matches, setMatches] = useState<OpenMatch[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const useMockMabar = import.meta.env.VITE_USE_MOCK_MABAR === 'true';
  const navigate = useNavigate();

  useEffect(() => {
    const loadMatches = async () => {
      try {
        setIsLoading(true);
        if (useMockMabar) {
          // Use mock data for visual QA
          setMatches([
            {
              id: '1', booking_id: 'b1', host_user_id: 'u1', host_name: 'Bima Aditya', title: 'FC Jakarta Casuals', sport_name: 'Mini Soccer',
              venue_name: 'GBK Alpha Field', court_name: 'Lapangan A', match_date: new Date().toISOString().split('T')[0], start_time: '19:00',
              end_time: '21:00', level: 'Beginner / Fun', max_players: 14, joined_count: 11, remaining_slots: 3, price_per_player: 45000,
              status: 'OPEN', created_at: '', updated_at: ''
            },
            {
              id: '2', booking_id: 'b2', host_user_id: 'u2', host_name: 'Dina Mariana', title: 'Smash Yuk', sport_name: 'Badminton Ganda',
              venue_name: 'Smash Arena', court_name: 'Court 2', match_date: new Date().toISOString().split('T')[0], start_time: '20:00',
              end_time: '22:00', level: 'Intermediate', max_players: 4, joined_count: 3, remaining_slots: 1, price_per_player: 35000,
              status: 'OPEN', created_at: '', updated_at: ''
            },
            {
              id: '3', booking_id: 'b3', host_user_id: 'u3', host_name: 'Anton S.', title: 'Hoops Weekend', sport_name: 'Basket (5v5)',
              venue_name: 'Kuningan Court', court_name: 'Indoor 1', match_date: '2026-06-25', start_time: '16:00',
              end_time: '18:00', level: 'All Levels', max_players: 10, joined_count: 6, remaining_slots: 4, price_per_player: 50000,
              status: 'OPEN', created_at: '', updated_at: ''
            }
          ]);
          setError(null);
          return;
        }

        const data = await fetchOpenMatches(1, 4);
        setMatches(data.data || []);
      } catch (err: any) {
        setError(err.message || 'Gagal memuat jadwal mabar');
      } finally {
        setIsLoading(false);
      }
    };

    loadMatches();
  }, [useMockMabar]);

  return (
    <section className="py-14 md:py-20 bg-white border-y border-gray-100">
      <div className="max-w-7xl mx-auto px-5 md:px-6">
      
        {/* Section Header */}
        <div className="flex flex-col md:flex-row justify-between items-start md:items-end mb-10 gap-4">
          <div>
            <h2 className="text-3xl md:text-4xl font-extrabold mb-2 text-text-main">Cari Lawan / Open Match</h2>
            <p className="text-base md:text-lg text-text-muted font-medium">
              Gabung pertandingan yang sedang butuh pemain di sekitarmu.
            </p>
          </div>
          
          <Button onClick={() => navigate('/open-matches')} variant="outline" className="shrink-0 font-bold border-gray-300">
            Lihat Semua Mabar
          </Button>
        </div>

        {/* States & Grid */}
        <div className="relative z-20 mt-8">
          {isLoading ? (
            <LoadingState message="Memuat jadwal mabar terdekat..." variant="cards" />
          ) : error ? (
            <ErrorState message={error} onRetry={() => window.location.reload()} />
          ) : matches.length === 0 && !useMockMabar ? (
            <EmptyState 
              title="Belum Ada Jadwal Mabar" 
              description="Jadilah yang pertama membuat jadwal mabar hari ini dan temukan teman baru untuk berolahraga!"
            />
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
              {matches.map((match) => (
                <MabarCard key={match.id} match={match} />
              ))}
            </div>
          )}
        </div>
      </div>
    </section>
  );
};
