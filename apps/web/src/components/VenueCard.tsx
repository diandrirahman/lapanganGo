import React from 'react';
import { Link } from 'react-router-dom';
import type { Venue } from '../types/venue';
import { ArrowUpRight, MapPin } from 'lucide-react';
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
    <Link to={`/venues/${venue.id}${playDate ? `?play_date=${playDate}` : ''}`} className="group flex h-full min-w-0 cursor-pointer flex-col rounded-[26px] border border-border-main/80 bg-bg-main p-3 transition-all duration-500 hover:-translate-y-1 hover:border-primary/25 hover:bg-white hover:shadow-lg">
      {/* Image Wrapper */}
      <div className="relative mb-5 flex aspect-[4/3] items-center justify-center overflow-hidden rounded-[20px] bg-gray-100">
        <SafeVenueImage 
          src={primaryPhotoUrl}
          venueId={venue.id}
          alt={venue.name}
          className="h-full w-full transition-transform duration-700 group-hover:scale-105"
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
      <div className="flex flex-1 flex-col px-1 pb-1">
        <div className="flex items-start justify-between gap-3">
          <h3 className="line-clamp-2 text-lg font-extrabold leading-tight text-text-main" title={venue.name}>
            {venue.name}
          </h3>
          <span className="grid h-9 w-9 shrink-0 place-items-center rounded-full border border-border-main bg-white text-text-main transition-all duration-300 group-hover:rotate-45 group-hover:border-primary group-hover:bg-primary group-hover:text-white">
            <ArrowUpRight className="h-4 w-4" />
          </span>
        </div>
        
        <div className="mb-4 mt-2 flex items-center gap-1 text-sm font-medium text-text-muted">
          <MapPin className="w-4 h-4 shrink-0" />
          <span className="line-clamp-1">{venue.address}, {venue.city}</span>
        </div>

        {/* Facilities Chips */}
        {venue.facilities.length > 0 && (
          <div className="mb-4 flex flex-wrap gap-1.5">
            {displayedFacilities.map(f => (
              <span key={f.id} className="rounded-full border border-border-main bg-white px-2.5 py-1 text-[11px] font-bold text-text-muted">
                {f.name}
              </span>
            ))}
            {extraFacilities > 0 && (
              <span className="rounded-full border border-border-main bg-white px-2.5 py-1 text-[11px] font-bold text-text-muted">
                +{extraFacilities}
              </span>
            )}
          </div>
        )}

        <p className="mt-auto border-t border-border-main/80 pt-4 text-xs font-extrabold uppercase tracking-[0.16em] text-primary">Lihat jadwal tersedia</p>
      </div>
    </Link>
  );
};
