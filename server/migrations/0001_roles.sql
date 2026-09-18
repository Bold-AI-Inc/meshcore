DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'mesh_gateway_role') THEN
        CREATE ROLE mesh_gateway_role LOGIN PASSWORD 'change_me_gateway';
    END IF;
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'mesh_admin_role') THEN
        CREATE ROLE mesh_admin_role LOGIN PASSWORD 'change_me_admin';
    END IF;
END
$$;

GRANT USAGE ON SCHEMA public TO mesh_gateway_role, mesh_admin_role;
