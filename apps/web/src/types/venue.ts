export interface Facility {
  id: string;
  name: string;
  icon?: string;
}

export interface Sport {
  id: string;
  name: string;
  description?: string;
}

export interface OperatingHour {
  id: string;
  court_id: string;
  day_of_week: number;
  open_time: string;
  close_time: string;
  is_closed: boolean;
}

export interface BlockedSlot {
  id: string;
  court_id: string;
  start_at: string;
  end_at: string;
  reason?: string;
  created_at?: string;
  updated_at?: string;
}

export interface VenuePhoto {
  id: string;
  venue_id: string;
  image_url: string;
  alt_text?: string;
  sort_order: number;
  is_primary: boolean;
  created_at: string;
}

export interface Venue {
  id: string;
  owner_profile_id?: string;
  name: string;
  description?: string;
  address: string;
  district?: string;
  city: string;
  province?: string;
  postal_code?: string;
  latitude?: number;
  longitude?: number;
  status?: string;
  primary_photo?: string;
  photos?: VenuePhoto[];
  facilities: Facility[];
  created_at: string;
  updated_at: string;
}

export interface Court {
  id: string;
  venue_id: string;
  sport: { id: string; name: string };
  name: string;
  description?: string;
  location_type: string;
  surface_type?: string | null;
  price_per_hour: number;
  status: string;
  created_at: string;
  updated_at: string;
}

export interface PublicCourt {
  id: string;
  sport: { id: string; name: string };
  name: string;
  description?: string;
  location_type: string;
  surface_type?: string | null;
  price_per_hour: number;
  created_at: string;
  updated_at: string;
}

export type VenueDetail = Venue & { courts: PublicCourt[] };
export type OwnerVenueDetail = Venue & { courts: Court[] };

export interface PublicVenuesResponse {
  venues: Venue[];
  page: number;
  limit: number;
}
