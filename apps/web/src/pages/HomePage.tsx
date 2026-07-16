import React from 'react';
import { HeroSection } from '../components/HeroSection';
import { TrustStatsSection } from '../components/TrustStatsSection';
import { VenueSection } from '../components/VenueSection';
import { MabarSection } from '../components/MabarSection';
import { FinalCtaSection } from '../components/FinalCtaSection';
import { PageShell } from '../components/layout/PageShell';
import { useAuth } from '../contexts/AuthContext';
import { Navigate } from 'react-router-dom';
import { getRoleHomeRoute } from '../lib/roleRouting';

export const HomePage: React.FC = () => {
  const { user, isLoading } = useAuth();

  if (isLoading) {
    return (
      <PageShell>
        <div className="pt-20 pb-40 text-center text-text-muted">Memuat...</div>
      </PageShell>
    );
  }

  const homeRoute = getRoleHomeRoute(user?.role);
  if (homeRoute !== '/') {
    return <Navigate to={homeRoute} replace />;
  }

  return (
    <PageShell>
      <div className="overflow-clip">
        <HeroSection />
        <TrustStatsSection />
        <VenueSection />
        <MabarSection />
        <FinalCtaSection />
      </div>
    </PageShell>
  );
};
