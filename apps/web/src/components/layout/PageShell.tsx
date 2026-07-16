import React from 'react';
import { Navbar } from '../Navbar';

interface PageShellProps {
  children: React.ReactNode;
}

export const PageShell: React.FC<PageShellProps> = ({ children }) => {
  return (
    <div className="min-h-screen flex flex-col relative bg-bg-main overflow-x-hidden">
      <Navbar />
      
      <main className="flex-1 pb-24 pt-[76px] md:pt-[108px]">
        {children}
      </main>

      <footer className="bg-surface py-10 mt-12 text-center text-text-muted font-medium border-t border-border-main">
        <div className="max-w-7xl mx-auto px-6">
          <p>&copy; {new Date().getFullYear()} LapangGo. Dirancang eksklusif dengan standar industri olahraga modern.</p>
        </div>
      </footer>
    </div>
  );
};
