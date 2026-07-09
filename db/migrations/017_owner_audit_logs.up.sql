CREATE TABLE owner_audit_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_profile_id UUID NOT NULL REFERENCES owner_profiles(id) ON DELETE CASCADE,
  actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  actor_role TEXT NOT NULL,
  action TEXT NOT NULL,
  entity_type TEXT NOT NULL,
  entity_id UUID,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  ip_address TEXT,
  user_agent TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_owner_audit_logs_owner_created
  ON owner_audit_logs(owner_profile_id, created_at DESC);

CREATE INDEX idx_owner_audit_logs_actor_created
  ON owner_audit_logs(actor_user_id, created_at DESC);

CREATE INDEX idx_owner_audit_logs_action_created
  ON owner_audit_logs(action, created_at DESC);

CREATE INDEX idx_owner_audit_logs_entity
  ON owner_audit_logs(entity_type, entity_id);
