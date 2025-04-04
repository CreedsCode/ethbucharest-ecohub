# Database Setup for Multi-tenant Authentication

This directory contains SQL scripts needed to properly set up the Supabase database for multi-tenant authentication.

## Required Functions and Policies

To ensure proper functioning of the authentication service, the following functions and policies must be created in your Supabase database:

### 1. `exec_sql` Function

This function allows executing arbitrary SQL from the application. It's used for tenant creation and management.

```sql
-- File: create_exec_sql.sql
CREATE OR REPLACE FUNCTION public.exec_sql(sql_query TEXT)
RETURNS TEXT AS $$
BEGIN
    EXECUTE sql_query;
    RETURN 'OK';
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

GRANT EXECUTE ON FUNCTION public.exec_sql TO authenticated;
GRANT EXECUTE ON FUNCTION public.exec_sql TO anon;
```

### 2. RLS Policies for users_tenants Table

These policies allow proper row-level security for the users_tenants table.

```sql
-- File: create_rls_policies.sql
CREATE POLICY users_tenants_insert_policy ON public.users_tenants
  FOR INSERT WITH CHECK (true);
  
CREATE POLICY users_tenants_update_policy ON public.users_tenants
  FOR UPDATE USING (true);
  
CREATE POLICY users_tenants_delete_policy ON public.users_tenants
  FOR DELETE USING (true);
```

### 3. User and Tenant Registration Function

This function streamlines the process of creating a tenant and linking a user to it.

```sql
-- File: register_user_and_tenant.sql
CREATE OR REPLACE FUNCTION register_user_and_tenant(user_id UUID, tenant_name TEXT)
RETURNS UUID AS $$
DECLARE
    new_tenant_id UUID;
BEGIN
    -- Create tenant
    INSERT INTO public.tenants (name)
    VALUES (tenant_name)
    RETURNING id INTO new_tenant_id;
    
    -- Link user to tenant
    INSERT INTO public.users_tenants (user_id, tenant_id, role)
    VALUES (user_id, new_tenant_id, 'admin');
    
    -- Create schema
    EXECUTE 'CREATE SCHEMA IF NOT EXISTS tenant_' || new_tenant_id;
    
    RETURN new_tenant_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

GRANT EXECUTE ON FUNCTION register_user_and_tenant TO authenticated;
GRANT EXECUTE ON FUNCTION register_user_and_tenant TO anon;
```

## How to Apply These Scripts

1. Login to your Supabase dashboard
2. Go to the SQL Editor
3. Copy and paste each script and run them individually
4. Verify the functions have been created by querying:

```sql
SELECT proname, prosecdef FROM pg_proc WHERE proname IN ('exec_sql', 'register_user_and_tenant');
```

5. Verify the RLS policies have been created by checking the Table Editor for the users_tenants table

## Troubleshooting

If you encounter issues with user-tenant relationships not being created:

1. Check if the `exec_sql` function exists and has SECURITY DEFINER privileges
2. Verify RLS policies exist on the users_tenants table
3. Ensure the service role has proper permissions
4. Check the tables exist with proper structure:
   - tenants
   - users_tenants 