import React from 'react';
import { Loader2 } from 'lucide-react';
import { cn } from '../../lib/utils';

interface LoadingStateProps extends React.HTMLAttributes<HTMLDivElement> {
  message?: string;
}

export const LoadingState = React.forwardRef<HTMLDivElement, LoadingStateProps>(
  ({ className, message = 'Memuat data...', ...props }, ref) => {
    return (
      <div 
        ref={ref}
        className={cn("flex flex-col items-center justify-center p-12 text-text-muted", className)}
        {...props}
      >
        <Loader2 className="w-10 h-10 animate-spin text-primary mb-4" />
        <p className="text-sm font-medium">{message}</p>
      </div>
    );
  }
);

LoadingState.displayName = 'LoadingState';
