import React from 'react';
import { cn } from '../../lib/utils';

interface BadgeProps extends React.HTMLAttributes<HTMLDivElement> {
  variant?: 'default' | 'success' | 'warning' | 'danger' | 'gradient';
}

export const Badge = React.forwardRef<HTMLDivElement, BadgeProps>(
  ({ className, variant = 'default', children, ...props }, ref) => {
    const baseStyles = 'inline-flex items-center justify-center px-3 py-1.5 rounded-full text-[13px] font-extrabold whitespace-nowrap shrink-0 transition-colors';
    
    const variants = {
      default: 'bg-text-main text-white',
      success: 'bg-green-100 text-green-700',
      warning: 'bg-yellow-100 text-yellow-700',
      danger: 'bg-red-100 text-red-700',
      gradient: 'bg-gradient-to-r from-[#FF512F] to-[#DD2476] text-white shadow-sm'
    };

    return (
      <div ref={ref} className={cn(baseStyles, variants[variant], className)} {...props}>
        {children}
      </div>
    );
  }
);

Badge.displayName = 'Badge';
