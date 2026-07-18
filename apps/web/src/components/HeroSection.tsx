import React, { useState } from 'react';
import { CalendarCheck, MapPin, Search } from 'lucide-react';
import { Button } from './ui/Button';
import { useNavigate } from 'react-router-dom';
import heroIndoorVenue from '../assets/hero-sports-venue.webp';

const heroBasketball = '/hero-basketball.webp';

export const HeroSection: React.FC = () => {
  const navigate = useNavigate();
  const [searchQuery, setSearchQuery] = useState('');
  const [sport, setSport] = useState('');

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    if (!searchQuery.trim() && !sport) {
      return;
    }

    const params = new URLSearchParams();
    if (searchQuery.trim()) {
      params.append('search', searchQuery.trim());
    }
    if (sport && sport !== 'Semua Olahraga') {
      params.append('sport', sport);
    }

    navigate(`/venues?${params.toString()}`);
  };

  const handleChipClick = (catName: string) => {
    navigate(`/venues?sport=${encodeURIComponent(catName)}`);
  };

  return (
    <section className="relative px-3 pb-12 sm:px-5 md:pb-20">
      <div className="relative mx-auto max-w-[1440px] overflow-hidden rounded-[28px] bg-[#05221e] shadow-[0_38px_90px_-50px_rgba(5,34,30,0.8)] sm:rounded-[40px] lg:min-h-[690px]">
        <div className="grid lg:min-h-[690px] lg:grid-cols-[45%_55%]">
          <div className="relative z-10 flex flex-col justify-between px-5 pb-10 pt-9 text-white sm:px-9 sm:pb-12 sm:pt-12 lg:px-12 lg:pb-[190px] lg:pt-16 xl:px-16">
            <div>
              <div className="animate-fade-up inline-flex items-center gap-2 rounded-full border border-white/20 bg-white/[0.06] px-4 py-2 text-[10px] font-extrabold uppercase tracking-[0.18em] text-white/80 sm:text-xs">
                Cari Venue Olahraga
              </div>

              <h1 className="animate-fade-up delay-75 mt-7 max-w-[680px] text-[clamp(3.35rem,6.4vw,6.4rem)] font-extrabold uppercase leading-[0.84] tracking-[-0.07em] text-white">
                Booking
                <span className="block">venue.</span>
                <span className="block text-secondary">Tanpa ribet.</span>
              </h1>
            </div>

            <div className="animate-fade-up delay-150 mt-10 flex max-w-xl flex-col gap-5 border-t border-white/15 pt-6 sm:flex-row sm:items-end sm:justify-between">
              <p className="max-w-md text-sm font-medium leading-relaxed text-white/65 sm:text-base">
                Cek jadwal real-time, pilih lapangan terbaik, dan amankan jam main tanpa bolak-balik chat admin.
              </p>
            </div>
          </div>

          <div className="animate-fade-up delay-150 relative min-h-[380px] overflow-hidden bg-[#0c4b46] sm:min-h-[500px] lg:min-h-full">
            <img
              src={heroBasketball}
              alt="Pertandingan basket bersama di lapangan terbuka"
              className="hero-shot hero-shot-one absolute inset-0 h-full w-full object-cover"
              decoding="async"
              fetchPriority="high"
            />
            <img
              src={heroIndoorVenue}
              alt="Venue futsal indoor modern"
              className="hero-shot hero-shot-two absolute inset-0 h-full w-full object-cover"
              decoding="async"
            />
            <div className="absolute inset-0 bg-gradient-to-t from-[#031b18]/70 via-[#042b27]/10 to-[#031b18]/20" />
            <div className="absolute inset-y-0 left-0 hidden w-36 bg-gradient-to-r from-[#05221e] to-transparent lg:block" />

            <div className="absolute left-5 top-5 rounded-full border border-white/25 bg-[#05221e]/60 px-4 py-2 text-[10px] font-extrabold uppercase tracking-[0.16em] text-white backdrop-blur-md sm:left-7 sm:top-7">
              Venue pilihan kota kamu
            </div>
          </div>
        </div>

        <form aria-label="Cari venue olahraga" onSubmit={handleSearch} className="animate-fade-up delay-200 relative z-20 mx-4 -mt-4 grid min-w-0 gap-2 rounded-[24px] border border-white/50 bg-white p-2 text-text-main shadow-[0_24px_60px_-28px_rgba(0,0,0,0.65)] sm:mx-8 md:grid-cols-[1fr_1fr_auto] md:items-center md:gap-0 md:rounded-full lg:absolute lg:bottom-[88px] lg:left-[4.5%] lg:right-[4.5%] lg:mx-0 lg:mt-0">
          <div className="flex min-w-0 flex-col px-4 py-2 text-left md:border-r md:border-gray-200 md:px-6">
            <label htmlFor="hero-location" className="mb-1 text-[10px] font-extrabold uppercase tracking-[0.16em] text-text-muted sm:text-xs">Lokasi / Venue</label>
            <input
              id="hero-location"
              type="text"
              placeholder="Contoh: Jakarta Selatan"
              className="w-full border-none bg-transparent text-[15px] font-bold text-text-main outline-none transition-all placeholder:font-medium placeholder:text-gray-400 focus:ring-0"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
            />
          </div>
          <div className="flex min-w-0 flex-col px-4 py-2 text-left md:px-6">
            <label htmlFor="hero-sport" className="mb-1 text-[10px] font-extrabold uppercase tracking-[0.16em] text-text-muted sm:text-xs">Olahraga</label>
            <select
              id="hero-sport"
              className="bg-transparent border-none text-[15px] font-bold text-text-main outline-none w-full min-w-0 cursor-pointer transition-colors"
              value={sport}
              onChange={(e) => setSport(e.target.value)}
            >
              <option value="">Semua Olahraga</option>
              <option value="Mini Soccer">Mini Soccer</option>
              <option value="Futsal">Futsal</option>
              <option value="Tenis Lapangan">Tenis Lapangan</option>
              <option value="Badminton">Badminton</option>
              <option value="Basket">Basket</option>
            </select>
          </div>
          <Button type="submit" className="h-13 w-full shrink-0 gap-2 p-0 font-bold active:scale-95 md:h-15 md:w-32 md:rounded-full">
            <Search className="w-5 h-5" />
            <span>Cari</span>
          </Button>
        </form>

        <div className="relative z-10 mt-4 flex flex-col gap-4 border-t border-white/10 px-5 py-5 sm:px-8 md:flex-row md:items-center md:justify-between lg:absolute lg:inset-x-0 lg:bottom-0 lg:mt-0 lg:px-12 xl:px-16">
          <div className="flex flex-wrap items-center gap-3 text-xs font-bold text-white/55">
            <span className="flex items-center gap-2 text-white"><CalendarCheck className="h-4 w-4 text-secondary" /> Slot ter-update</span>
            <span className="hidden h-1 w-1 rounded-full bg-white/25 sm:block" />
            <span className="flex items-center gap-2"><MapPin className="h-4 w-4 text-white/70" /> Venue terkurasi</span>
          </div>
          <div className="flex flex-wrap gap-2 animate-fade-up delay-300">
            {['Mini Soccer', 'Futsal', 'Tenis Lapangan', 'Badminton', 'Basket'].map((cat) => (
              <button
                key={cat}
                onClick={() => handleChipClick(cat)}
                type="button"
                className="rounded-full border border-white/15 bg-white/[0.07] px-3 py-1.5 text-[11px] font-bold text-white/70 transition-all hover:-translate-y-0.5 hover:border-secondary/50 hover:bg-secondary hover:text-white active:scale-95 sm:text-xs"
              >
                {cat}
              </button>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
};
