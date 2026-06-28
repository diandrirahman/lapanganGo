import React from 'react';
import { cn } from '../../lib/utils';

interface CardProps extends React.HTMLAttributes<HTMLDivElement> {}

export const Card = React.forwardRef<HTMLDivElement, CardProps>(
  ({ className, children, ...props }, ref) => {
    return (
      <div 
        ref={ref} 
        className={cn(
          "bg-surface rounded-2xl p-6 shadow-lg border border-border-main transition-transform duration-300 relative overflow-hidden group",
          className
        )} 
        {...props}
      >
        {children}
      </div>
    );
  }
);

Card.displayName = 'Card';
