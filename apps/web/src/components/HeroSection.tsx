import React, { useState } from 'react';
import { CalendarCheck, MapPin, Search, Trophy } from 'lucide-react';
import { Button } from './ui/Button';
import { useNavigate } from 'react-router-dom';

const heroSportsVenue = '/hero-basketball.webp';

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
    <section className="relative overflow-hidden bg-white pt-8 pb-12 md:pt-10 md:pb-16">
      <div className="max-w-7xl mx-auto px-4 md:px-6 grid lg:grid-cols-[0.92fr_1.08fr] gap-8 lg:gap-12 items-center">
        <div className="relative z-10 flex flex-col items-center text-center lg:items-start lg:text-left">
          <div className="animate-fade-up inline-flex items-center gap-2 px-4 py-1.5 bg-primary/10 text-primary rounded-full text-sm font-bold mb-5 border border-primary/15">
            <Trophy className="w-4 h-4" /> Temukan Venue Olahraga
          </div>
          
          <h1 className="animate-fade-up delay-75 text-[36px] sm:text-5xl lg:text-6xl font-extrabold tracking-tight mb-4 text-text-main leading-[1.08] max-w-[720px]">
            Booking Venue & Main Tanpa <span className="text-primary">Ribet.</span>
          </h1>
          
          <p className="animate-fade-up delay-150 text-base md:text-lg text-text-muted mb-6 md:mb-8 max-w-2xl leading-relaxed">
            Cari lapangan, cek jadwal real-time, dan mulai olahraga tanpa bolak-balik chat admin.
          </p>
          
          <form onSubmit={handleSearch} className="animate-fade-up delay-200 bg-white p-2 rounded-2xl md:rounded-full flex flex-col md:flex-row gap-2 md:gap-0 items-stretch md:items-center shadow-lg border border-gray-200 w-full max-w-3xl min-w-0 relative z-20">
            <div className="flex-1 min-w-0 flex flex-col px-4 md:px-6 py-2 md:border-r border-gray-200 w-full text-left">
              <label className="text-xs font-bold text-text-muted mb-1 uppercase tracking-wider">Lokasi / Venue</label>
              <input 
                type="text" 
                placeholder="Contoh: Jakarta Selatan" 
                className="bg-transparent border-none text-[15px] font-bold text-text-main outline-none w-full placeholder:text-gray-400 placeholder:font-medium transition-all focus:ring-0"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
              />
            </div>
            <div className="flex-1 min-w-0 flex flex-col px-4 md:px-6 py-2 w-full text-left">
              <label className="text-xs font-bold text-text-muted mb-1 uppercase tracking-wider">Olahraga</label>
              <select 
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
            <Button type="submit" className="w-full md:w-14 h-12 md:h-14 md:rounded-full p-0 flex items-center justify-center shrink-0 font-bold gap-2 active:scale-95 transition-transform">
              <Search className="w-5 h-5" />
              <span className="md:hidden">Cari Lapangan</span>
            </Button>
          </form>

          <div className="mt-6 flex gap-2 flex-wrap justify-center lg:justify-start animate-fade-up delay-300">
            {[
              'Mini Soccer', 
              'Futsal', 
              'Tenis Lapangan', 
              'Badminton', 
              'Basket'
            ].map((cat) => (
              <button 
                key={cat} 
                onClick={() => handleChipClick(cat)}
                type="button"
                className="bg-gray-50 border border-gray-200 px-3.5 sm:px-4 py-2 rounded-full font-bold text-xs sm:text-sm text-text-muted transition-all hover:bg-primary/10 hover:text-primary hover:border-primary/30 active:scale-95"
              >
                {cat}
              </button>
            ))}
          </div>
        </div>

        <div className="animate-fade-up delay-150 relative z-0 w-full max-w-full min-w-0">
          <div className="relative overflow-hidden rounded-3xl border border-white shadow-lg bg-gray-100 aspect-[16/10] min-h-[220px] w-full max-w-full">
            <img
              src={heroSportsVenue}
              alt="Lapangan olahraga indoor modern"
              className="h-full w-full object-cover"
              decoding="async"
              fetchPriority="high"
            />
            <div className="absolute inset-x-0 bottom-0 h-1/2 bg-gradient-to-t from-slate-950/55 to-transparent" />
            <div className="absolute left-4 right-4 bottom-4 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
              <div className="rounded-2xl bg-white/92 backdrop-blur px-4 py-3 shadow-sm border border-white/70 max-w-[260px]">
                <div className="flex items-center gap-2 text-xs font-bold text-primary uppercase tracking-wide">
                  <CalendarCheck className="w-4 h-4" />
                  Jadwal siap dicek
                </div>
                <p className="mt-1 text-sm font-extrabold text-text-main">Pilih venue, jam main, lalu booking.</p>
              </div>
              <div className="rounded-2xl bg-slate-950/75 text-white backdrop-blur px-4 py-3 border border-white/20">
                <div className="flex items-center gap-2 text-sm font-bold">
                  <MapPin className="w-4 h-4 text-primary" />
                  Venue pilihan kota Anda
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
};
