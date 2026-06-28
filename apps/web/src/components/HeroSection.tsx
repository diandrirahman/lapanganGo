import React, { useState } from 'react';
import { Search } from 'lucide-react';
import { Button } from './ui/Button';
import { useNavigate } from 'react-router-dom';

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
    <section className="pt-20 pb-20 relative overflow-hidden">
      <div className="max-w-7xl mx-auto px-6 grid lg:grid-cols-[1.1fr_0.9fr] gap-16 items-center">
        
        {/* Text Content */}
        <div className="text-left relative z-10">
          <div className="inline-flex items-center gap-2 px-5 py-2 bg-white text-primary rounded-full text-sm font-extrabold shadow-sm mb-6 border border-gray-100">
            <span className="text-base">🔥</span> Platform Olahraga No.1
          </div>
          
          <h1 className="text-[48px] md:text-[64px] lg:text-[76px] leading-[1.1] font-extrabold tracking-tight mb-6">
            Booking Venue & Main Tanpa <span className="bg-gradient-to-r from-[#FF512F] to-[#DD2476] text-transparent bg-clip-text">Ribet.</span>
          </h1>
          
          <p className="text-lg text-text-muted mb-10 max-w-[580px] leading-relaxed">
            Akses ribuan lapangan premium di seluruh Indonesia. Cek jadwal real-time, bayar instan, atau temukan teman mabar hari ini.
          </p>
          
          {/* Search Bar */}
          <form onSubmit={handleSearch} className="bg-surface p-3 rounded-full flex flex-col md:flex-row gap-2 items-center shadow-lg border border-gray-100 max-w-[800px] relative z-20">
            <div className="flex-1 flex flex-col px-6 py-2 border-r border-border-main w-full md:w-auto">
              <label className="text-[13px] font-extrabold text-text-main mb-1">Cari Lapangan / Lokasi</label>
              <input 
                type="text" 
                placeholder="Contoh: Jakarta Selatan" 
                className="bg-transparent border-none text-[15px] font-medium text-text-main outline-none w-full"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
              />
            </div>
            <div className="flex-1 flex flex-col px-6 py-2 w-full md:w-auto">
              <label className="text-[13px] font-extrabold text-text-main mb-1">Pilih Olahraga</label>
              <select 
                className="bg-transparent border-none text-[15px] font-medium text-text-main outline-none w-full cursor-pointer"
                value={sport}
                onChange={(e) => setSport(e.target.value)}
              >
                <option value="">Semua Olahraga</option>
                <option value="Mini Soccer">Mini Soccer</option>
                <option value="Futsal">Futsal</option>
                <option value="Tenis Lapangan">Tenis Lapangan</option>
                <option value="Badminton">Badminton</option>
              </select>
            </div>
            <Button type="submit" className="w-14 h-14 rounded-full p-0 flex items-center justify-center shrink-0 mt-2 md:mt-0 cursor-pointer">
              <Search className="w-6 h-6" />
            </Button>
          </form>

          {/* Categories */}
          <div className="mt-10 flex gap-3 flex-wrap">
            {[
              { icon: '⚽', label: 'Mini Soccer' }, 
              { icon: '🎾', label: 'Tenis' }, 
              { icon: '🏀', label: 'Basket' }, 
              { icon: '🏊‍♂️', label: 'Renang' }, 
              { icon: '🎱', label: 'Biliar' }
            ].map((cat) => (
              <button 
                key={cat.label} 
                onClick={() => handleChipClick(cat.label)}
                type="button"
                className="bg-surface border border-border-main px-5 py-2.5 rounded-full flex items-center gap-2.5 font-bold text-sm text-text-main transition-all shadow-sm hover:shadow-md hover:-translate-y-1 hover:border-primary/50 cursor-pointer"
              >
                {cat.icon} {cat.label}
              </button>
            ))}
          </div>
        </div>

        {/* Visuals (Dribbble style) */}
        <div className="relative h-[600px] hidden lg:block">
          <img 
            src="https://images.unsplash.com/photo-1518605368461-1ee790bbd105?q=80&w=1000&auto=format&fit=crop" 
            className="absolute right-0 top-5 w-[85%] h-[500px] rounded-[32px] object-cover shadow-2xl z-10" 
            alt="Sport Facility" 
          />
          
          {/* Floating Card 1 */}
          <div className="absolute left-0 top-24 z-20 bg-white/90 backdrop-blur-md p-5 rounded-2xl shadow-2xl flex gap-4 items-center border border-white animate-[float_4s_ease-in-out_infinite]">
            <div className="bg-primary-gradient p-3 rounded-xl text-white">
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3"><path d="M5 13l4 4L19 7" strokeLinecap="round" strokeLinejoin="round"/></svg>
            </div>
            <div>
              <div className="font-extrabold text-base">Booking Confirmed!</div>
              <div className="text-[13px] text-text-muted font-medium">GBK Field - 19:00 WIB</div>
            </div>
          </div>

          {/* Floating Card 2 */}
          <div className="absolute bottom-10 right-10 z-20 bg-surface px-6 py-4 rounded-full shadow-2xl flex items-center gap-3 font-bold border border-white animate-[float_5s_ease-in-out_infinite_reverse]">
            <div className="flex -space-x-3">
              <img src="https://images.unsplash.com/photo-1535713875002-d1d0cf377fde?q=80&w=100&auto=format&fit=crop" className="w-9 h-9 rounded-full border-2 border-white object-cover" />
              <img src="https://images.unsplash.com/photo-1494790108377-be9c29b29330?q=80&w=100&auto=format&fit=crop" className="w-9 h-9 rounded-full border-2 border-white object-cover" />
              <img src="https://images.unsplash.com/photo-1599566150163-29194dcaad36?q=80&w=100&auto=format&fit=crop" className="w-9 h-9 rounded-full border-2 border-white object-cover" />
            </div>
            <div>50K+ Active Players</div>
          </div>
        </div>

      </div>
    </section>
  );
};
