import React from 'react';
import { Link } from 'react-router-dom';
import type { Venue } from '../types/venue';
import { MapPin } from 'lucide-react';
import { SafeVenueImage } from './ui/SafeVenueImage';

interface Props {
  venue: Venue;
  playDate?: string;
}

export const VenueCard: React.FC<Props> = ({ venue, playDate }) => {
  // Show max 3 facilities
  const displayedFacilities = venue.facilities.slice(0, 3);
  const extraFacilities = venue.facilities.length - 3;

  let primaryPhotoUrl = venue.primary_photo;
  if (!primaryPhotoUrl && venue.photos && venue.photos.length > 0) {
    const firstPhoto = venue.photos[0];
    primaryPhotoUrl = typeof firstPhoto === 'string' ? firstPhoto : (firstPhoto as any).image_url;
  }

  return (
    <Link to={`/venues/${venue.id}${playDate ? `?play_date=${playDate}` : ''}`} className="animate-fade-up bg-white rounded-2xl p-4 flex flex-col group cursor-pointer border border-border-main shadow-sm hover:shadow-lg md:hover:-translate-y-1 hover:border-primary/30 transition-all duration-300">
      {/* Image Wrapper */}
      <div className="relative h-48 rounded-xl overflow-hidden mb-4 bg-gray-100 flex items-center justify-center">
        <SafeVenueImage 
          src={primaryPhotoUrl}
          venueId={venue.id}
          alt={venue.name}
          className="w-full h-full group-hover:scale-105 transition-transform duration-500"
          fallbackIcon="image"
        />
        {venue.has_promo && (
          <div className="absolute top-3 right-3 z-10">
            <span className="inline-flex items-center rounded-full bg-emerald-50/95 backdrop-blur-sm px-2.5 py-1 text-[11px] font-bold text-emerald-700 shadow-sm border border-emerald-100 uppercase tracking-wide">
              {playDate && venue.promos && venue.promos.length > 0 
                ? (venue.promos[0].discount_type === 'PERCENTAGE' 
                    ? `Diskon ${venue.promos[0].discount_value}%` 
                    : `Hemat Rp ${venue.promos[0].discount_value.toLocaleString('id-ID')}`) 
                : 'Ada Promo'}
            </span>
          </div>
        )}
      </div>

      {/* Content */}
      <div className="flex flex-col flex-1">
        <h3 className="text-lg font-bold mb-1 text-text-main line-clamp-1" title={venue.name}>
          {venue.name}
        </h3>
        
        <div className="flex items-center gap-1 text-text-muted text-sm font-medium mb-4">
          <MapPin className="w-4 h-4 shrink-0" />
          <span className="line-clamp-1">{venue.address}, {venue.city}</span>
        </div>

        {/* Facilities Chips */}
        {venue.facilities.length > 0 && (
          <div className="flex flex-wrap gap-2 mb-4">
            {displayedFacilities.map(f => (
              <span key={f.id} className="bg-bg-main text-text-muted px-2 py-1 rounded-md text-xs font-bold border border-border-main">
                {f.name}
              </span>
            ))}
            {extraFacilities > 0 && (
              <span className="bg-bg-main text-text-muted px-2 py-1 rounded-md text-xs font-bold border border-border-main">
                +{extraFacilities}
              </span>
            )}
          </div>
        )}

        {/* Action */}
        <div className="mt-auto pt-2">
          <button className="w-full rounded-xl py-3 bg-primary/10 text-primary font-bold text-sm transition-all duration-300 group-hover:bg-primary group-hover:text-white">
            Lihat Detail
          </button>
        </div>
      </div>
    </Link>
  );
};
