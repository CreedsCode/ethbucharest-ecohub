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

	// Create the query_sql function that returns JSON results
	sqlQuerySql := `
	CREATE OR REPLACE FUNCTION public.query_sql(sql_query TEXT)
	RETURNS JSON AS $$
	DECLARE
		result JSON;
	BEGIN
		EXECUTE 'SELECT array_to_json(array_agg(row_to_json(t))) FROM (' || sql_query || ') t' INTO result;
		RETURN COALESCE(result, '[]'::JSON);
	END;
	$$ LANGUAGE plpgsql SECURITY DEFINER;`

	// Run the SQL to create the query_sql function
	result = s.supabaseClient.Rpc(
		"exec_sql",
		"",
		map[string]interface{}{
			"sql_query": sqlQuerySql,
		},
	)
	if result == "" {
		fmt.Printf("Note: Could not create query_sql function\n")
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

	fmt.Printf("User registered successfully with ID: %s\n", user.ID.String())

	// 2. Create tenant using a function that returns the ID directly
	createTenantFunc := `
	CREATE OR REPLACE FUNCTION create_tenant_and_get_id(p_name TEXT) 
	RETURNS TEXT AS $$
	DECLARE
		new_id UUID;
	BEGIN
		INSERT INTO public.tenants (name) 
		VALUES (p_name) 
		RETURNING id INTO new_id;
		RETURN new_id::TEXT;
	END;
	$$ LANGUAGE plpgsql SECURITY DEFINER;`

	// Create the function
	s.supabaseClient.Rpc(
		"exec_sql",
		"",
		map[string]interface{}{
			"sql_query": createTenantFunc,
		},
	)

	// Call function to get tenant ID
	fmt.Printf("Attempting to create tenant with name: %s\n", tenantName)
	tenantSQL := fmt.Sprintf(`SELECT create_tenant_and_get_id('%s') as tenant_id`, tenantName)

	tenantResult := s.supabaseClient.Rpc(
		"query_sql",
		"",
		map[string]interface{}{
			"sql_query": tenantSQL,
		},
	)

	fmt.Printf("Tenant creation result: %s\n", tenantResult)

	// Parse the tenant ID from the JSON result
	var tenantArray []map[string]interface{}
	err = json.Unmarshal([]byte(tenantResult), &tenantArray)

	var tenantId string
	if err == nil && len(tenantArray) > 0 && tenantArray[0]["tenant_id"] != nil {
		tenantId = fmt.Sprintf("%v", tenantArray[0]["tenant_id"])
		fmt.Printf("Extracted tenant ID: %s\n", tenantId)
	} else {
		// If function doesn't work, try direct insert
		result, count, err := s.supabaseClient.From("tenants").
			Insert(map[string]interface{}{
				"name": tenantName,
			}, false, "", "id", "exact").
			Execute()

		if err == nil && count > 0 {
			var resp []map[string]interface{}
			if err := json.Unmarshal(result, &resp); err == nil && len(resp) > 0 {
				tenantId = fmt.Sprintf("%v", resp[0]["id"])
				fmt.Printf("Got tenant ID from direct insert: %s\n", tenantId)
			}
		}
	}

	if tenantId == "" {
		return "", fmt.Errorf("failed to get tenant ID")
	}

	fmt.Printf("Tenant created with ID: %s\n", tenantId)

	// 3. Link user to tenant as admin
	fmt.Printf("Attempting to link user %s to tenant %s\n", user.ID.String(), tenantId)

	// Create a function to link user to tenant
	linkFunc := `
	CREATE OR REPLACE FUNCTION link_user_to_tenant(
		p_user_id UUID, 
		p_tenant_id UUID, 
		p_role TEXT
	) RETURNS TEXT AS $$
	DECLARE
		link_id UUID;
	BEGIN
		INSERT INTO public.users_tenants (user_id, tenant_id, role)
		VALUES (p_user_id, p_tenant_id, p_role)
		RETURNING id INTO link_id;
		RETURN link_id::TEXT;
	END;
	$$ LANGUAGE plpgsql SECURITY DEFINER;`

	// Create the function
	s.supabaseClient.Rpc(
		"exec_sql",
		"",
		map[string]interface{}{
			"sql_query": linkFunc,
		},
	)

	// Call the function to link user
	linkSQL := fmt.Sprintf(`SELECT link_user_to_tenant('%s', '%s', 'admin') as link_id`,
		user.ID.String(), tenantId)

	linkResult := s.supabaseClient.Rpc(
		"query_sql",
		"",
		map[string]interface{}{
			"sql_query": linkSQL,
		},
	)

	fmt.Printf("User-tenant link result: %s\n", linkResult)

	var linkId string
	var linkArray []map[string]interface{}
	err = json.Unmarshal([]byte(linkResult), &linkArray)
	if err == nil && len(linkArray) > 0 && linkArray[0]["link_id"] != nil {
		linkId = fmt.Sprintf("%v", linkArray[0]["link_id"])
		fmt.Printf("User linked to tenant with link ID: %s\n", linkId)
	} else {
		// Try direct insert if function fails
		_, count, err := s.supabaseClient.From("users_tenants").
			Insert(map[string]interface{}{
				"user_id":   user.ID.String(),
				"tenant_id": tenantId,
				"role":      "admin",
			}, false, "", "id", "exact").
			Execute()

		if err == nil && count > 0 {
			fmt.Printf("User linked to tenant via direct insert\n")
		} else {
			fmt.Printf("WARNING: Could not link user to tenant: %v\n", err)
		}
	}

	// 4. Create schema for tenant data
	schemaName := fmt.Sprintf("tenant_%s", tenantId)
	fmt.Printf("Creating schema: %s\n", schemaName)

	schemaResult := s.supabaseClient.Rpc(
		"create_tenant_schema",
		"",
		map[string]interface{}{
			"schema_name": schemaName,
		},
	)

	fmt.Printf("Schema creation result: %s\n", schemaResult)

	return user.ID.String(), nil
}

// Helper function to create the users_tenants table
func (s *AuthService) createUsersTenantTable() error {
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
	result := s.supabaseClient.Rpc(
		"exec_sql",
		"",
		map[string]interface{}{
			"sql_query": sqlUsersTenants,
		},
	)
	if result == "" {
		return fmt.Errorf("failed to create users_tenants table")
	}
	return nil
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

// GetUserWithTenantInfo retrieves user information along with associated tenant and role
func (s *AuthService) GetUserWithTenantInfo(userId string) (map[string]interface{}, error) {
	// SQL query to join user, tenant, and role information
	joinSQL := fmt.Sprintf(`
		SELECT 
			u.id as user_id, 
			u.email, 
			u.raw_user_meta_data->>'first_name' as first_name,
			u.raw_user_meta_data->>'last_name' as last_name,
			t.id as tenant_id, 
			t.name as tenant_name, 
			ut.role 
		FROM 
			auth.users u
		LEFT JOIN 
			public.users_tenants ut ON u.id = ut.user_id::uuid
		LEFT JOIN 
			public.tenants t ON ut.tenant_id = t.id
		WHERE 
			u.id = '%s'::uuid
	`, userId)

	fmt.Printf("Executing query with query_sql: %s\n", joinSQL)

	// Execute the query using our new query_sql function that returns JSON results
	queryResult := s.supabaseClient.Rpc(
		"query_sql",
		"",
		map[string]interface{}{
			"sql_query": joinSQL,
		},
	)

	fmt.Printf("Query result: %s\n", queryResult)

	// Parse the JSON array result (should be an array with one object)
	var userInfoArray []map[string]interface{}
	err := json.Unmarshal([]byte(queryResult), &userInfoArray)
	if err != nil {
		fmt.Printf("Error parsing query result: %v\n", err)
		return map[string]interface{}{"user_id": userId, "error": "Failed to parse user info"}, nil
	}

	// If we got a result, return the first item
	if len(userInfoArray) > 0 {
		return userInfoArray[0], nil
	}

	// If we didn't get a result from the join, try querying just the user
	fmt.Printf("No joined results found, querying just the user\n")
	userSQL := fmt.Sprintf(`
		SELECT 
			id as user_id, 
			email, 
			raw_user_meta_data->>'first_name' as first_name,
			raw_user_meta_data->>'last_name' as last_name
		FROM 
			auth.users 
		WHERE 
			id = '%s'::uuid
	`, userId)

	userResult := s.supabaseClient.Rpc(
		"query_sql",
		"",
		map[string]interface{}{
			"sql_query": userSQL,
		},
	)

	fmt.Printf("User query result: %s\n", userResult)

	// Parse the user result
	var userArray []map[string]interface{}
	err = json.Unmarshal([]byte(userResult), &userArray)
	if err != nil || len(userArray) == 0 {
		return map[string]interface{}{"user_id": userId, "error": "User not found"}, nil
	}

	// Start with basic user info
	userInfo := userArray[0]

	// Now try to find tenants for this user
	tenantSQL := fmt.Sprintf(`
		SELECT 
			t.id as tenant_id, 
			t.name as tenant_name,
			ut.role
		FROM 
			public.users_tenants ut 
		JOIN 
			public.tenants t ON ut.tenant_id = t.id
		WHERE 
			ut.user_id = '%s'::uuid
	`, userId)

	tenantResult := s.supabaseClient.Rpc(
		"query_sql",
		"",
		map[string]interface{}{
			"sql_query": tenantSQL,
		},
	)

	fmt.Printf("Tenant query result: %s\n", tenantResult)

	var tenantArray []map[string]interface{}
	err = json.Unmarshal([]byte(tenantResult), &tenantArray)
	if err == nil && len(tenantArray) > 0 {
		// Add tenant information to the user info
		userInfo["tenant_id"] = tenantArray[0]["tenant_id"]
		userInfo["tenant_name"] = tenantArray[0]["tenant_name"]
		userInfo["role"] = tenantArray[0]["role"]

		// If there are multiple tenants, add them as an array
		if len(tenantArray) > 1 {
			userInfo["tenants"] = tenantArray
		}
	}

	return userInfo, nil
}
