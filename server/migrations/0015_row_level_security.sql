ALTER TABLE admins ENABLE ROW LEVEL SECURITY;
ALTER TABLE admins FORCE ROW LEVEL SECURITY;
CREATE POLICY admin_manage_admins ON admins
    FOR ALL TO mesh_admin_role USING (true) WITH CHECK (true);

ALTER TABLE admin_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE admin_sessions FORCE ROW LEVEL SECURITY;
CREATE POLICY admin_manage_sessions ON admin_sessions
    FOR ALL TO mesh_admin_role USING (true) WITH CHECK (true);

ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;
CREATE POLICY gateway_read_users ON users
    FOR SELECT TO mesh_gateway_role USING (true);
CREATE POLICY admin_manage_users ON users
    FOR ALL TO mesh_admin_role USING (true) WITH CHECK (true);

ALTER TABLE providers ENABLE ROW LEVEL SECURITY;
ALTER TABLE providers FORCE ROW LEVEL SECURITY;
CREATE POLICY gateway_read_providers ON providers
    FOR SELECT TO mesh_gateway_role USING (true);
CREATE POLICY admin_manage_providers ON providers
    FOR ALL TO mesh_admin_role USING (true) WITH CHECK (true);

ALTER TABLE provider_hourly_usage ENABLE ROW LEVEL SECURITY;
ALTER TABLE provider_hourly_usage FORCE ROW LEVEL SECURITY;
CREATE POLICY gateway_write_provider_hourly_usage ON provider_hourly_usage
    FOR INSERT TO mesh_gateway_role WITH CHECK (true);
CREATE POLICY gateway_update_provider_hourly_usage ON provider_hourly_usage
    FOR UPDATE TO mesh_gateway_role USING (true) WITH CHECK (true);
CREATE POLICY gateway_read_provider_hourly_usage ON provider_hourly_usage
    FOR SELECT TO mesh_gateway_role USING (true);
CREATE POLICY admin_read_provider_hourly_usage ON provider_hourly_usage
    FOR SELECT TO mesh_admin_role USING (true);

ALTER TABLE models ENABLE ROW LEVEL SECURITY;
ALTER TABLE models FORCE ROW LEVEL SECURITY;
CREATE POLICY gateway_read_models ON models
    FOR SELECT TO mesh_gateway_role USING (true);
CREATE POLICY admin_manage_models ON models
    FOR ALL TO mesh_admin_role USING (true) WITH CHECK (true);

ALTER TABLE user_model_access ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_model_access FORCE ROW LEVEL SECURITY;
CREATE POLICY gateway_read_user_model_access ON user_model_access
    FOR SELECT TO mesh_gateway_role USING (true);
CREATE POLICY admin_manage_user_model_access ON user_model_access
    FOR ALL TO mesh_admin_role USING (true) WITH CHECK (true);

ALTER TABLE mesh_api_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE mesh_api_keys FORCE ROW LEVEL SECURITY;
CREATE POLICY gateway_read_keys ON mesh_api_keys
    FOR SELECT TO mesh_gateway_role USING (true);
CREATE POLICY admin_manage_keys ON mesh_api_keys
    FOR ALL TO mesh_admin_role USING (true) WITH CHECK (true);

ALTER TABLE user_model_policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_model_policies FORCE ROW LEVEL SECURITY;
CREATE POLICY gateway_read_user_model_policies ON user_model_policies
    FOR SELECT TO mesh_gateway_role USING (true);
CREATE POLICY admin_manage_user_model_policies ON user_model_policies
    FOR ALL TO mesh_admin_role USING (true) WITH CHECK (true);

ALTER TABLE daily_spend ENABLE ROW LEVEL SECURITY;
ALTER TABLE daily_spend FORCE ROW LEVEL SECURITY;
CREATE POLICY gateway_write_daily_spend ON daily_spend
    FOR INSERT TO mesh_gateway_role WITH CHECK (true);
CREATE POLICY gateway_update_daily_spend ON daily_spend
    FOR UPDATE TO mesh_gateway_role USING (true) WITH CHECK (true);
CREATE POLICY gateway_read_daily_spend ON daily_spend
    FOR SELECT TO mesh_gateway_role USING (true);
CREATE POLICY admin_read_daily_spend ON daily_spend
    FOR SELECT TO mesh_admin_role USING (true);

-- request_logs: gateway only ever appends; retention deletes and reads run
-- as the admin role.
ALTER TABLE request_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE request_logs FORCE ROW LEVEL SECURITY;
CREATE POLICY gateway_write_request_logs ON request_logs
    FOR INSERT TO mesh_gateway_role WITH CHECK (true);
CREATE POLICY admin_read_request_logs ON request_logs
    FOR SELECT TO mesh_admin_role USING (true);
CREATE POLICY admin_delete_request_logs ON request_logs
    FOR DELETE TO mesh_admin_role USING (true);

ALTER TABLE admin_audit_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE admin_audit_log FORCE ROW LEVEL SECURITY;
CREATE POLICY admin_write_audit_log ON admin_audit_log
    FOR INSERT TO mesh_admin_role WITH CHECK (true);
CREATE POLICY admin_read_audit_log ON admin_audit_log
    FOR SELECT TO mesh_admin_role USING (true);

ALTER TABLE user_model_hourly_usage ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_model_hourly_usage FORCE ROW LEVEL SECURITY;
CREATE POLICY gateway_write_user_model_hourly_usage ON user_model_hourly_usage
    FOR INSERT TO mesh_gateway_role WITH CHECK (true);
CREATE POLICY gateway_update_user_model_hourly_usage ON user_model_hourly_usage
    FOR UPDATE TO mesh_gateway_role USING (true) WITH CHECK (true);
CREATE POLICY gateway_read_user_model_hourly_usage ON user_model_hourly_usage
    FOR SELECT TO mesh_gateway_role USING (true);
CREATE POLICY admin_read_user_model_hourly_usage ON user_model_hourly_usage
    FOR SELECT TO mesh_admin_role USING (true);
