import React from 'react';
import { MapPin, ShieldCheck, Clock, Users } from 'lucide-react';

export const TrustStatsSection: React.FC = () => {
  const stats = [
    {
      icon: <MapPin className="w-6 h-6 text-primary" />,
      title: "Banyak Pilihan Venue",
      description: "Temukan lapangan terbaik di sekitarmu dengan mudah."
    },
    {
      icon: <ShieldCheck className="w-6 h-6 text-primary" />,
      title: "Booking Aman & Cepat",
      description: "Konfirmasi instan tanpa harus chat admin venue."
    },
    {
      icon: <Users className="w-6 h-6 text-primary" />,
      title: "Mabar Lebih Seru",
      description: "Gabung open match dan tambah teman olahraga baru."
    },
    {
      icon: <Clock className="w-6 h-6 text-primary" />,
      title: "Main Kapan Saja",
      description: "Jadwal real-time dan fleksibel sesuai waktumu."
    }
  ];

  return (
    <section className="py-10 md:py-12 bg-white border-y border-gray-100 relative z-20">
      <div className="max-w-7xl mx-auto px-5 md:px-6">
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3 md:gap-4">
          {stats.map((stat, idx) => (
            <div 
              key={idx} 
              className="animate-fade-up group flex items-start gap-4 rounded-2xl border border-gray-100 bg-white p-5 text-left hover:-translate-y-0.5 hover:border-primary/20 hover:shadow-md transition-all duration-300"
              style={{ animationDelay: `${idx * 100}ms` }}
            >
              <div className="w-12 h-12 bg-primary/10 rounded-2xl flex items-center justify-center shrink-0 transition-transform group-hover:scale-105">
                {stat.icon}
              </div>
              <div>
                <h3 className="text-base font-extrabold text-text-main mb-1">{stat.title}</h3>
                <p className="text-sm text-text-muted font-medium leading-relaxed">{stat.description}</p>
              </div>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
};
