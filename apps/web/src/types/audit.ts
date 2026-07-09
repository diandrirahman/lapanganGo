export interface AuditActor {
  id?: string;
  name?: string;
  email?: string;
  role: string;
}

export interface AuditLog {
  id: string;
  actor: AuditActor;
  action: string;
  entity_type: string;
  entity_id?: string;
  metadata: Record<string, any>;
  ip_address?: string;
  user_agent?: string;
  created_at: string;
}

export interface AuditLogQuery {
  action?: string;
  entity_type?: string;
  actor_user_id?: string;
  start_date?: string;
  end_date?: string;
  page?: number;
  limit?: number;
}
