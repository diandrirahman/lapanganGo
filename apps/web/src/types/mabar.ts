export interface OpenMatch {
  id: string;
  booking_id: string;
  host_user_id: string;
  host_name: string;
  title: string;
  description?: string;
  sport_name: string;
  venue_name: string;
  court_name: string;
  match_date: string;
  start_time: string;
  end_time: string;
  level: string;
  max_players: number;
  joined_count: number;
  remaining_slots: number;
  price_per_player: number;
  status: 'OPEN' | 'FULL' | 'CANCELLED';
  created_at: string;
  updated_at: string;
}

export interface ParticipantResponse {
  id: string;
  user_id: string;
  name: string;
  status: string;
  joined_at: string;
}

export interface OpenMatchDetailResponse {
  open_match: OpenMatch;
  participants: ParticipantResponse[];
}

export interface OpenMatchesResponse {
  open_matches: OpenMatch[];
}
