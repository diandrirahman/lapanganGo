import React from 'react';
import { HeroSection } from '../components/HeroSection';
import { VenueSection } from '../components/VenueSection';
import { MabarSection } from '../components/MabarSection';
import { PageShell } from '../components/layout/PageShell';

export const HomePage: React.FC = () => {
  return (
    <PageShell>
      <HeroSection />
      <VenueSection />
      <MabarSection />
    </PageShell>
  );
};
