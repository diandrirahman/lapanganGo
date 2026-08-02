import React from 'react';
import { RotateCcw, Sparkles } from 'lucide-react';
import { isPortfolioDemo } from '../demo/config';
import { resetPortfolioDemoState } from '../demo/state';

export const PortfolioDemoBanner: React.FC = () => {
  if (!isPortfolioDemo) return null;

  const resetDemo = () => {
    resetPortfolioDemoState();
    window.location.replace('/login');
  };

  return (
    <aside className="fixed inset-x-0 bottom-0 z-[100] border-t border-amber-300 bg-amber-100 px-4 py-2 text-amber-950 shadow-[0_-8px_30px_rgba(15,23,42,0.12)]" role="status">
      <div className="mx-auto flex max-w-7xl flex-col items-center justify-between gap-2 text-center sm:flex-row sm:text-left">
        <div className="flex items-center gap-2 text-xs font-bold sm:text-sm">
          <Sparkles className="h-4 w-4 shrink-0" />
          <span>Portfolio Demo Mode · data sintetis · tidak memproses transaksi nyata</span>
        </div>
        <button type="button" onClick={resetDemo} className="inline-flex items-center gap-1.5 rounded-full border border-amber-500 bg-white px-3 py-1.5 text-xs font-extrabold transition hover:bg-amber-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-amber-600">
          <RotateCcw className="h-3.5 w-3.5" /> Reset Demo
        </button>
      </div>
    </aside>
  );
};
