import React from 'react';
import { MapPin, ShieldCheck, Clock, Users } from 'lucide-react';
import { ScrollReveal } from './ui/ScrollReveal';

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
    <section className="relative z-20 bg-bg-main py-14 md:py-24">
      <div className="mx-auto max-w-7xl px-5 md:px-6">
        <ScrollReveal className="grid gap-8 lg:grid-cols-[0.95fr_1.05fr] lg:items-end">
          <div>
            <p className="mb-4 text-xs font-extrabold uppercase tracking-[0.2em] text-primary">Satu tempat, semua beres</p>
            <h2 className="max-w-3xl text-[clamp(2.5rem,6vw,5.25rem)] font-extrabold leading-[0.95] tracking-[-0.055em] text-text-main">
              Lebih banyak main. Lebih sedikit menunggu.
            </h2>
          </div>
          <p className="max-w-xl text-base font-medium leading-relaxed text-text-muted lg:justify-self-end lg:text-lg">
            Dari mencari lapangan sampai menemukan teman mabar, LapangGo membuat setiap langkah terasa singkat, jelas, dan aman.
          </p>
        </ScrollReveal>

        <div className="mt-12 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:mt-16 lg:grid-cols-4 lg:gap-4">
          {stats.map((stat, idx) => (
            <ScrollReveal key={stat.title} delay={idx * 80} className="h-full">
              <div className="group flex h-full min-h-52 flex-col justify-between rounded-[24px] border border-border-main/80 bg-white p-5 text-left transition-all duration-300 hover:-translate-y-1 hover:border-primary/25 hover:shadow-lg md:p-6">
                <div className="flex items-center justify-between">
                  <div className="grid h-12 w-12 shrink-0 place-items-center rounded-2xl bg-primary/10 transition-transform duration-300 group-hover:rotate-3 group-hover:scale-105">
                    {stat.icon}
                  </div>
                  <span className="text-xs font-extrabold tracking-[0.18em] text-text-muted/60">0{idx + 1}</span>
                </div>
                <div className="mt-10">
                  <h3 className="mb-2 text-lg font-extrabold leading-tight text-text-main">{stat.title}</h3>
                  <p className="text-sm font-medium leading-relaxed text-text-muted">{stat.description}</p>
                </div>
              </div>
            </ScrollReveal>
          ))}
        </div>
      </div>
    </section>
  );
};
