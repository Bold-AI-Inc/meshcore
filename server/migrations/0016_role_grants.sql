-- RLS policies filter which rows a role sees; the role still needs the
-- underlying table privilege to attempt the operation at all.

-- mesh_admin_role: full DML on everything the admin API manages.
GRANT SELECT, INSERT, UPDATE, DELETE ON admins TO mesh_admin_role;
GRANT SELECT, INSERT, UPDATE, DELETE ON admin_sessions TO mesh_admin_role;
GRANT SELECT, INSERT, UPDATE, DELETE ON users TO mesh_admin_role;
GRANT SELECT, INSERT, UPDATE, DELETE ON providers TO mesh_admin_role;
GRANT SELECT, INSERT, UPDATE, DELETE ON models TO mesh_admin_role;
GRANT SELECT, INSERT, UPDATE, DELETE ON user_model_access TO mesh_admin_role;
GRANT SELECT, INSERT, UPDATE, DELETE ON mesh_api_keys TO mesh_admin_role;
GRANT SELECT, INSERT, UPDATE, DELETE ON user_model_policies TO mesh_admin_role;

-- read-only for tables the gateway owns writing to; DELETE on request_logs
-- is the retention job, which runs as the admin role.
GRANT SELECT ON provider_hourly_usage TO mesh_admin_role;
GRANT SELECT ON daily_spend TO mesh_admin_role;
GRANT SELECT ON user_model_hourly_usage TO mesh_admin_role;
GRANT SELECT, DELETE ON request_logs TO mesh_admin_role;

-- audit log: admin API writes and reads it; needs sequence access for the
-- BIGSERIAL id column's implicit nextval() on insert.
GRANT SELECT, INSERT ON admin_audit_log TO mesh_admin_role;
GRANT USAGE, SELECT ON SEQUENCE admin_audit_log_id_seq TO mesh_admin_role;

-- mesh_gateway_role: scoped to what the request-proxying gateway needs.
GRANT SELECT ON users TO mesh_gateway_role;
GRANT SELECT ON providers TO mesh_gateway_role;
GRANT SELECT ON models TO mesh_gateway_role;
GRANT SELECT ON user_model_access TO mesh_gateway_role;
GRANT SELECT ON mesh_api_keys TO mesh_gateway_role;
GRANT SELECT ON user_model_policies TO mesh_gateway_role;
GRANT SELECT, INSERT, UPDATE ON provider_hourly_usage TO mesh_gateway_role;
GRANT SELECT, INSERT, UPDATE ON daily_spend TO mesh_gateway_role;
GRANT SELECT, INSERT, UPDATE ON user_model_hourly_usage TO mesh_gateway_role;
GRANT SELECT, INSERT ON request_logs TO mesh_gateway_role;
