import React from 'react';
import { cn } from '../../lib/utils';
import { Loader2 } from 'lucide-react';

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'secondary' | 'ghost' | 'outline' | 'danger';
  size?: 'sm' | 'md' | 'lg';
  isLoading?: boolean;
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = 'primary', size = 'md', isLoading = false, children, disabled, ...props }, ref) => {
    const baseStyles = 'inline-flex items-center justify-center font-bold rounded-full transition-all duration-300 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed';
    
    const variants = {
      primary: 'bg-primary text-white hover:bg-primary/90 hover:shadow-md hover:-translate-y-1',
      secondary: 'bg-white text-text-main border border-border-main shadow-sm hover:bg-gray-50',
      ghost: 'bg-transparent text-text-main hover:bg-gray-100',
      outline: 'bg-transparent border-2 border-primary text-primary hover:bg-primary/5',
      danger: 'bg-red-500 text-white hover:bg-red-600 shadow-sm'
    };

    const sizes = {
      sm: 'py-2 px-4 text-xs',
      md: 'py-3 px-6 text-sm',
      lg: 'py-4 px-8 text-base'
    };

    return (
      <button
        ref={ref}
        className={cn(baseStyles, variants[variant], sizes[size], className)}
        disabled={disabled || isLoading}
        {...props}
      >
        {isLoading && <Loader2 className="w-4 h-4 mr-2 animate-spin" />}
        {children}
      </button>
    );
  }
);

Button.displayName = 'Button';
