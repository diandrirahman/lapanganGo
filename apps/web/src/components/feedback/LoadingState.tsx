import React from 'react';
import { Loader2 } from 'lucide-react';
import { cn } from '../../lib/utils';

interface LoadingStateProps extends React.HTMLAttributes<HTMLDivElement> {
  message?: string;
  variant?: 'default' | 'cards';
}

export const LoadingState = React.forwardRef<HTMLDivElement, LoadingStateProps>(
  ({ className, message = 'Memuat data...', variant = 'default', ...props }, ref) => {
    if (variant === 'cards') {
      return (
        <div
          ref={ref}
          className={cn("grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6 animate-fade-in", className)}
          aria-label={message}
          {...props}
        >
          {[0, 1, 2].map((item) => (
            <div key={item} className="rounded-2xl border border-border-main bg-white p-4 shadow-sm">
              <div className="h-44 rounded-xl animate-shimmer mb-4" />
              <div className="h-5 w-2/3 rounded-md animate-shimmer mb-3" />
              <div className="h-4 w-full rounded-md animate-shimmer mb-2" />
              <div className="h-4 w-1/2 rounded-md animate-shimmer mb-5" />
              <div className="h-11 rounded-xl animate-shimmer" />
            </div>
          ))}
        </div>
      );
    }

    return (
      <div 
        ref={ref}
        className={cn("flex flex-col items-center justify-center p-12 text-text-muted animate-fade-in", className)}
        {...props}
      >
        <Loader2 className="w-10 h-10 animate-spin text-primary mb-4" />
        <p className="text-sm font-medium">{message}</p>
      </div>
    );
  }
);

LoadingState.displayName = 'LoadingState';
