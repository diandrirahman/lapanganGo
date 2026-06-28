import React from 'react';
import { AlertCircle } from 'lucide-react';
import { cn } from '../../lib/utils';
import { Button } from '../ui/Button';

interface ErrorStateProps extends React.HTMLAttributes<HTMLDivElement> {
  title?: string;
  message: string;
  onRetry?: () => void;
}

export const ErrorState = React.forwardRef<HTMLDivElement, ErrorStateProps>(
  ({ className, title = 'Oops! Terjadi Kesalahan', message, onRetry, ...props }, ref) => {
    return (
      <div 
        ref={ref}
        className={cn("bg-red-50 border border-red-100 rounded-2xl p-10 text-center shadow-sm max-w-lg mx-auto flex flex-col items-center", className)}
        {...props}
      >
        <AlertCircle className="w-12 h-12 text-red-500 mb-4" />
        <h3 className="text-xl font-bold text-red-700 mb-2">{title}</h3>
        <p className="text-red-500 mb-6">{message}</p>
        {onRetry && (
          <Button variant="danger" onClick={onRetry}>
            Coba Lagi
          </Button>
        )}
      </div>
    );
  }
);

ErrorState.displayName = 'ErrorState';
