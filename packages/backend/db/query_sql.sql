-- Create a SQL function that returns query results as JSON
CREATE OR REPLACE FUNCTION public.query_sql(sql_query TEXT)
RETURNS JSONB AS $$
DECLARE
    result JSONB;
BEGIN
    EXECUTE 'SELECT array_to_json(array_agg(row_to_json(t))) FROM (' || sql_query || ') t' INTO result;
    RETURN COALESCE(result, '[]'::JSONB);
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- Grant usage to authenticated and anon users
GRANT EXECUTE ON FUNCTION public.query_sql TO authenticated;
GRANT EXECUTE ON FUNCTION public.query_sql TO anon; 