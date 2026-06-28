import React from 'react';
import type { OpenMatch } from '../types/mabar';
import { Card } from './ui/Card';
import { Badge } from './ui/Badge';
import { Button } from './ui/Button';
import { useNavigate } from 'react-router-dom';
import { formatRupiah } from '../lib/utils';

interface Props {
  match: OpenMatch;
}

export const MabarCard: React.FC<Props> = ({ match }) => {
  const navigate = useNavigate();
	const formatDate = (dateString: string, timeString: string) => {
    try {
      const date = new Date(dateString);
      const isToday = new Date().toDateString() === date.toDateString();
      const dateText = isToday 
        ? "Hari Ini" 
        : date.toLocaleDateString('id-ID', { day: 'numeric', month: 'short' });
      return `${dateText}, ${timeString} WIB`;
    } catch {
      return `${dateString}, ${timeString}`;
    }
  };

  const isFull = match.status === 'FULL';
  const isCancelled = match.status === 'CANCELLED';

  return (
    <Card className="group flex flex-col h-full">
      {/* Header */}
      <div className="flex justify-between items-start mb-5 pb-4 border-b border-border-main">
        <div className="flex gap-3 items-center">
          {/* Avatar Placeholder */}
          <div className="w-12 h-12 rounded-full border-2 border-primary p-0.5 shrink-0 bg-secondary-soft flex items-center justify-center font-bold text-primary">
            {match.host_name.charAt(0).toUpperCase()}
          </div>
          <div className="flex flex-col gap-0.5">
            <h4 className="text-[16px] font-extrabold text-text-main line-clamp-1" title={match.title}>{match.title}</h4>
            <p className="text-[13px] text-text-muted font-medium">Host: {match.host_name}</p>
          </div>
        </div>
        
        {/* Status Badge */}
        {isCancelled ? (
          <Badge variant="danger">Dibatalkan</Badge>
        ) : isFull ? (
          <Badge>Slot Penuh</Badge>
        ) : (
          <Badge variant="gradient">Sisa {match.remaining_slots} Slot</Badge>
        )}
      </div>

      {/* Info List */}
      <div className="space-y-3.5 mb-6 flex-1">
        <div className="flex justify-between text-[14px]">
          <span className="text-text-muted">Olahraga</span>
          <strong className="font-bold text-text-main text-right">{match.sport_name}</strong>
        </div>
        <div className="flex justify-between items-center text-[14px]">
          <span className="text-text-muted shrink-0">Lokasi</span>
          <strong className="font-bold text-text-main text-right truncate max-w-[65%]" title={`${match.venue_name} - ${match.court_name}`}>
            {match.venue_name} - {match.court_name}
          </strong>
        </div>
        <div className="flex justify-between text-[14px]">
          <span className="text-text-muted">Waktu</span>
          <strong className="font-bold text-text-main text-right">
            {formatDate(match.match_date, match.start_time)}
          </strong>
        </div>
        <div className="flex justify-between text-[14px]">
          <span className="text-text-muted">Level</span>
          <strong className="font-bold text-text-main text-right">{match.level}</strong>
        </div>
        <div className="flex justify-between text-[14px]">
          <span className="text-text-muted">Patungan</span>
          <strong className="font-bold text-text-main text-right">
            {match.price_per_player > 0 ? `${formatRupiah(match.price_per_player)} / Org` : 'Gratis'}
          </strong>
        </div>
      </div>

      {/* Action Button */}
      <Button 
        onClick={() => navigate(`/open-matches/${match.id}`)}
        variant="secondary"
        className="w-full mt-auto"
      >
        Lihat Detail
      </Button>
    </Card>
  );
};
