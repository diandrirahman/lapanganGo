import React from 'react';
import { ArrowRight } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import { Button } from './ui/Button';
import { ScrollReveal } from './ui/ScrollReveal';

export const FinalCtaSection: React.FC = () => {
  const navigate = useNavigate();

  return (
    <section className="relative z-20 bg-bg-main px-3 py-16 sm:px-5 md:py-24">
      <ScrollReveal className="relative mx-auto max-w-[1440px] overflow-hidden rounded-[30px] bg-primary px-5 py-16 text-center text-white shadow-float sm:rounded-[40px] sm:px-10 md:py-24">
        <div className="pointer-events-none absolute -left-20 -top-20 h-64 w-64 rounded-full border-[50px] border-white/10" />
        <div className="pointer-events-none absolute -bottom-24 -right-20 h-72 w-72 rounded-full bg-[#071c1a]/15" />
        <div className="relative mx-auto max-w-5xl">
          <p className="mb-5 text-xs font-extrabold uppercase tracking-[0.2em] text-white/70">Waktunya bergerak</p>
          <h2 className="text-[clamp(2.8rem,7.5vw,6.7rem)] font-extrabold leading-[0.9] tracking-[-0.06em]">
            Lapangan sudah siap. Kamu kapan?
          </h2>
          <p className="mx-auto mb-8 mt-6 max-w-2xl text-base font-medium leading-relaxed text-white/75 md:text-lg">
            Venue pilihan dan jadwal mabar siap kamu jelajahi. Pilih waktu, ajak tim, lalu mulai main.
          </p>
          <Button
            onClick={() => navigate('/venues')}
            variant="secondary"
            className="mx-auto flex gap-2 border-0 bg-white px-8 py-3 text-base font-bold text-text-main shadow-md transition-all hover:-translate-y-0.5 hover:bg-[#071c1a] hover:text-white hover:shadow-lg active:scale-95"
          >
            Cari Lapangan Sekarang <ArrowRight className="h-5 w-5" />
          </Button>
        </div>
      </ScrollReveal>
    </section>
  );
};
