import React, { useState, useEffect } from 'react';
import { Building2, Image as ImageIcon } from 'lucide-react';
import { getPlaceholderImage } from '../../lib/utils';

interface SafeVenueImageProps {
  src?: string | null;
  venueId: string;
  alt: string;
  className?: string;
  fallbackIcon?: 'building' | 'image';
}

export const SafeVenueImage: React.FC<SafeVenueImageProps> = ({ 
  src, 
  venueId, 
  alt, 
  className = '', 
  fallbackIcon = 'image' 
}) => {
  const defaultPlaceholder = getPlaceholderImage(venueId);
  const [imgSrc, setImgSrc] = useState<string>(src || defaultPlaceholder);
  const [errorCount, setErrorCount] = useState(0);

  // Reset effect when src or venueId changes
  useEffect(() => {
    setImgSrc(src || getPlaceholderImage(venueId));
    setErrorCount(0);
  }, [src, venueId]);

  const handleImageError = () => {
    if (errorCount === 0 && src) {
      // First fallback: try placeholder
      setImgSrc(getPlaceholderImage(venueId));
      setErrorCount(1);
    } else {
      // Final fallback: show icon block
      setImgSrc('');
      setErrorCount(2);
    }
  };

  if (!imgSrc) {
    return (
      <div className={`bg-gray-100 flex items-center justify-center ${className}`}>
        {fallbackIcon === 'building' ? (
          <Building2 className="w-10 h-10 text-gray-300" />
        ) : (
          <ImageIcon className="w-10 h-10 text-gray-300" />
        )}
      </div>
    );
  }

  return (
    <img 
      src={imgSrc} 
      alt={alt} 
      onError={handleImageError}
      className={`object-cover ${className}`}
    />
  );
};
