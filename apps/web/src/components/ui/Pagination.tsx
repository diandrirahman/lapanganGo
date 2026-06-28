import React from 'react';
import { ChevronLeft, ChevronRight } from 'lucide-react';

interface PaginationProps {
  page: number;
  totalPages: number;
  onPageChange: (page: number) => void;
}

export const Pagination: React.FC<PaginationProps> = ({ page, totalPages, onPageChange }) => {
  if (totalPages <= 1) return null;

  return (
    <div className="flex items-center justify-center space-x-2 mt-8">
      <button
        onClick={() => onPageChange(page - 1)}
        disabled={page <= 1}
        className="p-2 rounded-md border border-border bg-surface text-text-secondary disabled:opacity-50 disabled:cursor-not-allowed hover:bg-surface-hover transition-colors"
        aria-label="Previous page"
      >
        <ChevronLeft className="w-5 h-5" />
      </button>

      <div className="flex items-center space-x-1">
        {Array.from({ length: totalPages }, (_, i) => i + 1).map((p) => {
          // Simple logic: show first, last, and current +/- 1
          if (
            p === 1 ||
            p === totalPages ||
            (p >= page - 1 && p <= page + 1)
          ) {
            return (
              <button
                key={p}
                onClick={() => onPageChange(p)}
                className={`w-10 h-10 rounded-md border text-sm font-medium transition-colors ${
                  p === page
                    ? 'bg-primary text-white border-primary'
                    : 'border-border bg-surface text-text hover:bg-surface-hover'
                }`}
              >
                {p}
              </button>
            );
          } else if (
            (p === page - 2 && page > 3) ||
            (p === page + 2 && page < totalPages - 2)
          ) {
            return (
              <span key={p} className="w-10 h-10 flex items-center justify-center text-text-muted">
                ...
              </span>
            );
          }
          return null;
        })}
      </div>

      <button
        onClick={() => onPageChange(page + 1)}
        disabled={page >= totalPages}
        className="p-2 rounded-md border border-border bg-surface text-text-secondary disabled:opacity-50 disabled:cursor-not-allowed hover:bg-surface-hover transition-colors"
        aria-label="Next page"
      >
        <ChevronRight className="w-5 h-5" />
      </button>
    </div>
  );
};
