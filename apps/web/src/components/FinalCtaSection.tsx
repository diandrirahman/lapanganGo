import React from 'react';
import { ArrowRight } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import { Button } from './ui/Button';

export const FinalCtaSection: React.FC = () => {
  const navigate = useNavigate();

  return (
    <section className="py-14 md:py-20 bg-bg-main relative z-20">
      <div className="max-w-4xl mx-auto px-5 md:px-6 text-center">
        <h2 className="text-3xl md:text-4xl font-extrabold text-text-main mb-3">
          Siap untuk Berkeringat Hari Ini?
        </h2>
        <p className="text-base md:text-lg text-text-muted font-medium mb-7 max-w-2xl mx-auto">
          Venue pilihan dan jadwal mabar siap kamu jelajahi. Jangan tunda olahragamu.
        </p>
        <Button 
          onClick={() => navigate('/venues')} 
          className="font-bold px-8 py-3 rounded-full text-base flex items-center gap-2 mx-auto shadow-md hover:shadow-lg hover:-translate-y-0.5 active:scale-95 transition-all"
        >
          Cari Lapangan Sekarang <ArrowRight className="w-5 h-5" />
        </Button>
      </div>
    </section>
  );
};
