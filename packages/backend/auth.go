package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/supabase-community/gotrue-go/types"
	"github.com/supabase-community/supabase-go"
)

type AuthService struct {
	supabaseClient *supabase.Client
	jwtSecret      string
}

func NewAuthService() *AuthService {
	supabaseUrl := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_SERVICE_KEY")
	jwtSecret := os.Getenv("JWT_SECRET")

	client, err := supabase.NewClient(supabaseUrl, supabaseKey, nil)
	if err != nil {
		panic(fmt.Errorf("failed to create Supabase client: %w", err))
	}

	// Initialize the database
	service := &AuthService{
		supabaseClient: client,
		jwtSecret:      jwtSecret,
	}

	err = service.initDatabase()
	if err != nil {
		fmt.Printf("Warning: Failed to initialize database: %v\n", err)
	}

	return service
}

// initDatabase creates the necessary tables if they don't exist
func (s *AuthService) initDatabase() error {
	// Create tenants table
	sqlTenants := `
	CREATE TABLE IF NOT EXISTS public.tenants (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		name TEXT NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL
	);`

	// Run the SQL to create the tenants table
	result := s.supabaseClient.Rpc(
		"exec_sql",
		"",
		map[string]interface{}{
			"sql_query": sqlTenants,
		},
	)
	if result == "" {
		return fmt.Errorf("failed to create tenants table")
	}

	// Create users_tenants table
	sqlUsersTenants := `
	CREATE TABLE IF NOT EXISTS public.users_tenants (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		user_id UUID NOT NULL,
		tenant_id UUID NOT NULL REFERENCES public.tenants(id),
		role TEXT NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
		UNIQUE(user_id, tenant_id)
	);`

	// Run the SQL to create the users_tenants table
	result = s.supabaseClient.Rpc(
		"exec_sql",
		"",
		map[string]interface{}{
			"sql_query": sqlUsersTenants,
		},
	)
	if result == "" {
		return fmt.Errorf("failed to create users_tenants table")
	}

	// Create the tenant schema function
	sqlCreateTenantSchema := `
	CREATE OR REPLACE FUNCTION public.create_tenant_schema(schema_name TEXT)
	RETURNS TEXT AS $$
	BEGIN
		EXECUTE 'CREATE SCHEMA IF NOT EXISTS ' || schema_name;
		RETURN schema_name;
	END;
	$$ LANGUAGE plpgsql SECURITY DEFINER;`

	// Run the SQL to create the function
	result = s.supabaseClient.Rpc(
		"exec_sql",
		"",
		map[string]interface{}{
			"sql_query": sqlCreateTenantSchema,
		},
	)
	if result == "" {
		return fmt.Errorf("failed to create tenant schema function")
	}

	// Create the exec_sql function if it doesn't exist
	sqlExecSql := `
	CREATE OR REPLACE FUNCTION public.exec_sql(sql_query TEXT)
	RETURNS TEXT AS $$
	BEGIN
		EXECUTE sql_query;
		RETURN 'OK';
	END;
	$$ LANGUAGE plpgsql SECURITY DEFINER;`

	// Run the SQL to create the exec_sql function
	result = s.supabaseClient.Rpc(
		"exec_sql",
		"",
		map[string]interface{}{
			"sql_query": sqlExecSql,
		},
	)
	if result == "" {
		// This is expected to fail the first time since the function doesn't exist yet
		fmt.Printf("Note: Could not execute the exec_sql function (expected initially)\n")
	}

	return nil
}

// RegisterUser creates a user and their tenant
func (s *AuthService) RegisterUser(email, password, tenantName string) (string, error) {
	// 1. Register user with Supabase
	user, err := s.supabaseClient.Auth.Signup(types.SignupRequest{
		Email:    email,
		Password: password,
	})
	if err != nil {
		return "", fmt.Errorf("failed to register user: %w", err)
	}

	// 2. Create tenant - try direct SQL if the Insert operation fails
	var tenantId string

	// Try direct SQL insert first for reliability
	sqlInsertTenant := fmt.Sprintf("INSERT INTO public.tenants (name) VALUES ('%s') RETURNING id", tenantName)
	rpcResult := s.supabaseClient.Rpc(
		"exec_sql",
		"",
		map[string]interface{}{
			"sql_query": sqlInsertTenant,
		},
	)

	if rpcResult != "" {
		// Direct SQL insert worked
		fmt.Printf("Tenant created via SQL: %s\n", rpcResult)
		// Extract the UUID from the result
		tenantId = rpcResult
	} else {
		// Fall back to the standard API
		var tenant struct {
			ID string `json:"id"`
		}
		var count int64
		result, count, err := s.supabaseClient.From("tenants").Insert(
			map[string]interface{}{"name": tenantName}, // data
			false,   // upsert
			"",      // onConflict
			"id",    // returning
			"exact", // count
		).Single().Execute()

		fmt.Printf("Tenant creation result: %+v, count: %d\n", string(result), count)

		if err != nil {
			return "", fmt.Errorf("failed to create tenant: %w", err)
		}

		if count == 0 {
			return "", fmt.Errorf("no tenant created (check if 'tenants' table exists)")
		}

		if len(result) == 0 {
			return "", fmt.Errorf("empty result when creating tenant")
		}

		if err := json.Unmarshal(result, &tenant); err != nil {
			return "", fmt.Errorf("failed to unmarshal tenant: %w, raw result: %s", err, string(result))
		}

		tenantId = tenant.ID
	}

	// 3. Link user to tenant as admin via direct SQL
	sqlInsertUserTenant := fmt.Sprintf(
		"INSERT INTO public.users_tenants (user_id, tenant_id, role) VALUES ('%s', '%s', 'admin') RETURNING id",
		user.ID.String(), tenantId,
	)

	userTenantResult := s.supabaseClient.Rpc(
		"exec_sql",
		"",
		map[string]interface{}{
			"sql_query": sqlInsertUserTenant,
		},
	)

	if userTenantResult == "" {
		return "", fmt.Errorf("failed to link user to tenant")
	}

	// 4. Create schema for tenant data
	schemaResult := s.supabaseClient.Rpc(
		"create_tenant_schema", // function name
		"",                     // count
		map[string]interface{}{ // params
			"schema_name": "tenant_" + tenantId,
		},
	)
	if schemaResult == "" {
		return "", fmt.Errorf("failed to create tenant schema (check if 'create_tenant_schema' function exists)")
	}

	return user.ID.String(), nil
}

// GenerateMetabaseToken creates a JWT for Metabase SSO
func (s *AuthService) GenerateMetabaseToken(userId string) (string, error) {
	// 1. Get user's tenant info via direct SQL
	sqlUserTenant := fmt.Sprintf(
		"SELECT tenant_id, role FROM public.users_tenants WHERE user_id = '%s' LIMIT 1",
		userId,
	)

	userTenantResult := s.supabaseClient.Rpc(
		"exec_sql",
		"",
		map[string]interface{}{
			"sql_query": sqlUserTenant,
		},
	)

	if userTenantResult == "" {
		return "", fmt.Errorf("no user tenant found")
	}

	// Parse the result - direct SQL responses are harder to parse
	var userTenant struct {
		TenantID string `json:"tenant_id"`
		Role     string `json:"role"`
	}

	// If we have trouble parsing the direct SQL result, fall back to the API
	var err error
	if strings.Contains(userTenantResult, "tenant_id") {
		err = json.Unmarshal([]byte(userTenantResult), &userTenant)
	}

	if err != nil || userTenant.TenantID == "" {
		result, count, err := s.supabaseClient.From("users_tenants").
			Select("tenant_id,role", "", false).
			Eq("user_id", userId).
			Single().
			Execute()
		if err != nil {
			return "", fmt.Errorf("failed to get user tenant: %w", err)
		}
		if count == 0 {
			return "", fmt.Errorf("no user tenant found")
		}
		if err := json.Unmarshal(result, &userTenant); err != nil {
			return "", fmt.Errorf("failed to unmarshal user tenant: %w", err)
		}
	}

	// 2. Get user details from auth.users
	sqlUser := fmt.Sprintf(
		"SELECT email, raw_user_meta_data->>'first_name' as first_name, raw_user_meta_data->>'last_name' as last_name FROM auth.users WHERE id = '%s' LIMIT 1",
		userId,
	)

	userResult := s.supabaseClient.Rpc(
		"exec_sql",
		"",
		map[string]interface{}{
			"sql_query": sqlUser,
		},
	)

	var user struct {
		Email     string `json:"email"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	}

	// If we have trouble with direct SQL, fall back to the API
	err = json.Unmarshal([]byte(userResult), &user)
	if err != nil || user.Email == "" {
		result, count, err := s.supabaseClient.From("auth.users").
			Select("email,raw_user_meta_data->>'first_name' as first_name,raw_user_meta_data->>'last_name' as last_name", "", false).
			Eq("id", userId).
			Single().
			Execute()
		if err != nil {
			return "", fmt.Errorf("failed to get user details: %w", err)
		}
		if count == 0 {
			return "", fmt.Errorf("no user found")
		}
		if err := json.Unmarshal(result, &user); err != nil {
			return "", fmt.Errorf("failed to unmarshal user details: %w", err)
		}
	}

	// 3. Create Metabase JWT claims
	claims := jwt.MapClaims{
		// Required by Metabase
		"email":      user.Email,
		"first_name": user.FirstName,
		"last_name":  user.LastName,
		"exp":        time.Now().Add(time.Hour * 24).Unix(),
		"iat":        time.Now().Unix(),

		// Custom attributes for data sandboxing
		"tenant_id": userTenant.TenantID,
		"role":      userTenant.Role,

		// Group membership (optional)
		"groups": []string{"tenant_" + userTenant.TenantID + "_" + userTenant.Role},
	}

	// 4. Generate JWT
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(s.jwtSecret))
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return signedToken, nil
}
