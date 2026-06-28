import React from 'react';
import type { PublicCourt } from '../types/venue';
import { Card } from './ui/Card';

interface Props {
  court: PublicCourt;
  onSelect?: (courtId: string) => void;
}

export const CourtCard: React.FC<Props> = ({ court, onSelect }) => {
  return (
    <Card className="p-5 flex flex-col justify-between border-border-main hover:border-primary/30 transition-colors">
      <div className="mb-4">
        <div className="flex justify-between items-start mb-2">
          <h4 className="text-[18px] font-bold text-text-main">{court.name}</h4>
          <span className="text-[12px] font-bold px-2 py-1 bg-green-100 text-green-700 rounded-md">
            {court.location_type}
          </span>
        </div>
        <p className="text-[14px] text-text-muted font-medium mb-1">
          Jenis: <span className="text-text-main font-bold">{court.sport?.name || '-'}</span>
        </p>
        <p className="text-[14px] text-text-muted font-medium">
          Harga: <span className="text-text-main font-bold">Rp {court.price_per_hour.toLocaleString('id-ID')} / Jam</span>
        </p>
      </div>

      <button 
        onClick={() => onSelect?.(court.id)}
        className="w-full bg-primary/10 hover:bg-primary text-primary hover:text-white font-bold py-3 rounded-xl transition-all"
      >
        Pilih Jadwal
      </button>
    </Card>
  );
};
