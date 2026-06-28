import React from 'react';
import { Navbar } from '../Navbar';

interface PageShellProps {
  children: React.ReactNode;
}

export const PageShell: React.FC<PageShellProps> = ({ children }) => {
  return (
    <div className="min-h-screen flex flex-col relative">
      <div className="bg-mesh"></div>
      <Navbar />
      
      <main className="flex-1 pb-24 pt-[120px]">
        {children}
      </main>

      <footer className="bg-surface py-12 mt-24 text-center text-text-muted font-medium border-t border-border-main">
        <div className="max-w-7xl mx-auto px-6">
          <p>&copy; {new Date().getFullYear()} LapanganGo. Dirancang eksklusif dengan standar industri olahraga modern.</p>
        </div>
      </footer>
    </div>
  );
};
