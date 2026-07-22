import React, { useEffect, useState } from 'react';
import type { OpenMatch } from '../types/mabar';
import { fetchOpenMatches } from '../lib/api';
import { MabarCard } from './MabarCard';
import { Button } from './ui/Button';
import { EmptyState } from './feedback/EmptyState';
import { ErrorState } from './feedback/ErrorState';
import { LoadingState } from './feedback/LoadingState';
import { useNavigate } from 'react-router-dom';
import { ArrowRight } from 'lucide-react';
import { ScrollReveal } from './ui/ScrollReveal';

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
    <section className="relative overflow-hidden bg-[#071c1a] py-16 text-white md:py-28">
      <div className="pointer-events-none absolute -right-28 top-24 h-80 w-80 rounded-full bg-primary/25 blur-3xl" />
      <div className="relative mx-auto max-w-7xl px-5 md:px-6">

        {/* Section Header */}
        <ScrollReveal className="mb-10 flex flex-col items-start justify-between gap-6 md:mb-14 md:flex-row md:items-end">
          <div>
            <p className="mb-4 text-xs font-extrabold uppercase tracking-[0.2em] text-secondary">Main Bareng</p>
            <h2 className="max-w-4xl text-[clamp(2.6rem,6vw,5.4rem)] font-extrabold leading-[0.95] tracking-[-0.055em] text-white">Datang sendiri. Pulang bawa tim.</h2>
            <p className="mt-5 text-base font-medium text-white/60 md:text-lg">
              Gabung pertandingan yang sedang butuh pemain di sekitarmu.
            </p>
          </div>

          <Button onClick={() => navigate('/open-matches')} variant="secondary" className="group shrink-0 gap-2 border-white/15 bg-white/10 text-white hover:bg-white hover:text-text-main">
            Lihat Semua Mabar <ArrowRight className="h-4 w-4 transition-transform group-hover:translate-x-1" />
          </Button>
        </ScrollReveal>

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
            <div className="grid grid-cols-1 gap-5 md:grid-cols-2 lg:grid-cols-3">
              {matches.map((match, index) => (
                <ScrollReveal key={match.id} delay={index * 80} className="h-full">
                  <MabarCard match={match} />
                </ScrollReveal>
              ))}
            </div>
          )}
        </div>
      </div>
    </section>
  );
};
