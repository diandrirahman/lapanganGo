import React from 'react';
import { FolderOpen } from 'lucide-react';
import { cn } from '../../lib/utils';
import { Button } from '../ui/Button';

interface EmptyStateProps extends React.HTMLAttributes<HTMLDivElement> {
  title: string;
  description: string;
  icon?: React.ReactNode;
  actionLabel?: string;
  onAction?: () => void;
}

export const EmptyState = React.forwardRef<HTMLDivElement, EmptyStateProps>(
  ({ className, title, description, icon, actionLabel, onAction, ...props }, ref) => {
    return (
      <div 
        ref={ref}
        className={cn("bg-surface border border-border-main rounded-2xl p-8 text-center shadow-lg max-w-lg mx-auto flex flex-col items-center", className)}
        {...props}
      >
        <div className="text-gray-400 mb-4 bg-gray-50 p-4 rounded-full">
          {icon || <FolderOpen className="w-12 h-12" />}
        </div>
        <h3 className="text-xl font-bold text-text-main mb-2">{title}</h3>
        <p className="text-text-muted text-sm max-w-sm mx-auto mb-6">
          {description}
        </p>
        {actionLabel && onAction && (
          <Button onClick={onAction}>
            {actionLabel}
          </Button>
        )}
      </div>
    );
  }
);

EmptyState.displayName = 'EmptyState';
