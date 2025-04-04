import { useEffect, useState } from 'react';
import { useApi } from '../contexts/ApiContext';

interface DashboardProps {
  showDataIngestion?: boolean;
}

interface TenantService {
  id: string;
  tenant_id: string;
  service_type: string;
  container_id: string | null;
  target_identifier: string;
  last_processed_id: string;
  status: string;
  config: Record<string, unknown>;
  metadata: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export default function Dashboard({ showDataIngestion = true }: DashboardProps) {
  const { user, fetchData } = useApi();
  const [dashboardUrl, setDashboardUrl] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [services, setServices] = useState<TenantService[]>([]);
  const [loadingServices, setLoadingServices] = useState(false);

  useEffect(() => {
    const getMetabaseToken = async () => {
      if (!user?.tenant_id) {
        setError('User tenant information not available');
        setLoading(false);
        return;
      }

      try {
        const response = await fetchData<{ signed_url: string }>('/metabase/dashboard', {
          method: 'POST',
          body: JSON.stringify({
            tenant_id: user.tenant_id
          })
        });
        
        setDashboardUrl(response.signed_url);
        setLoading(false);
      } catch (err) {
        console.error('Failed to fetch Metabase dashboard URL:', err);
        setError('Failed to load dashboard. Please try again later.');
        setLoading(false);
      }
    };

    getMetabaseToken();
  }, [user, fetchData]);

  useEffect(() => {
    const fetchServices = async () => {
      if (!user?.tenant_id) return;

      setLoadingServices(true);
      try {
        const response = await fetchData<TenantService[]>('/services', {
          method: 'POST',
          body: JSON.stringify({
            tenant_id: user.tenant_id
          })
        });
        
        setServices(response);
      } catch (err) {
        console.error('Failed to fetch services:', err);
      } finally {
        setLoadingServices(false);
      }
    };

    fetchServices();
    // Refresh services every 15 seconds
    const interval = setInterval(fetchServices, 15000);
    return () => clearInterval(interval);
  }, [user, fetchData]);

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'running':
        return 'text-green-700 bg-green-100';
      case 'stopped':
        return 'text-yellow-700 bg-yellow-100';
      case 'failed':
        return 'text-red-700 bg-red-100';
      default:
        return 'text-gray-700 bg-gray-100';
    }
  };

  // Calculate a percentage based on processed blocks (for EVM indexers)
  const calculateProgress = (service: TenantService) => {
    if (service.service_type !== 'evm-indexer') return 0;
    
    const lastProcessedId = parseInt(service.last_processed_id || '0');
    const startBlock = service.metadata?.start_time ? 1 : 0; // Assume some progress if we have a start time
    
    // For demo purposes, assume we're 75% done once we've processed some blocks
    if (lastProcessedId > 0) return 75;
    
    return startBlock > 0 ? 10 : 0; // Show some progress if we have a start block
  };

  const getServiceName = (service: TenantService) => {
    switch (service.service_type) {
      case 'evm-indexer':
        return `Contract ${service.target_identifier.substring(0, 10)}...`;
      default:
        return `${service.service_type} - ${service.target_identifier.substring(0, 10)}...`;
    }
  };

  const getServiceDetails = (service: TenantService) => {
    switch (service.service_type) {
      case 'evm-indexer': {
        const blockNumber = service.last_processed_id ? parseInt(service.last_processed_id) : 0;
        return blockNumber > 0 
          ? `Processing block ${blockNumber}`
          : 'Initializing...';
      }
      default:
        return `Last processed: ${service.last_processed_id || 'None'}`;
    }
  };

  return (
    <div className="min-h-screen bg-white flex flex-col">
      <header className="bg-[#0F172A] text-white p-4">
        <div className="max-w-7xl mx-auto flex items-center justify-between">
          <div>
            <h1 className="text-xl font-bold">{user?.tenant_name || 'Web3 Insights'}</h1>
            <p className="text-gray-300 text-sm">{user?.email}</p>
          </div>
        </div>
      </header>

      <main className="flex-grow p-6 max-w-7xl mx-auto w-full">
        {showDataIngestion && (
          <div className="mb-6">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-lg font-medium text-[#0F172A]">Active Services</h2>
              <button 
                onClick={async () => {
                  if (!user?.tenant_id) return;
                  setLoadingServices(true);
                  try {
                    await fetchData('/services/start', {
                      method: 'POST',
                      body: JSON.stringify({
                        tenant_id: user.tenant_id
                      })
                    });
                    
                    // Refetch services
                    const response = await fetchData<TenantService[]>('/services', {
                      method: 'POST',
                      body: JSON.stringify({
                        tenant_id: user.tenant_id
                      })
                    });
                    
                    setServices(response);
                  } catch (err) {
                    console.error('Failed to start services:', err);
                  } finally {
                    setLoadingServices(false);
                  }
                }}
                disabled={loadingServices}
                className="px-4 py-2 bg-blue-600 text-white rounded-lg text-sm font-medium hover:bg-blue-700 disabled:opacity-50"
              >
                {loadingServices ? 'Starting...' : 'Start All'}
              </button>
            </div>
            
            {services.length === 0 ? (
              <div className="bg-gray-50 p-4 rounded-lg border border-gray-200 text-center">
                <p className="text-gray-600">No services configured yet. Add a smart contract to get started.</p>
              </div>
            ) : (
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {services.map(service => (
                  <div key={service.id} className="bg-blue-50 p-4 rounded-lg border border-blue-100">
                    <div className="flex justify-between items-start mb-2">
                      <h3 className="font-medium text-blue-800">
                        {getServiceName(service)}
                      </h3>
                      <div className="flex items-center space-x-2">
                        <span className="text-xs bg-blue-200 text-blue-800 px-2 py-1 rounded-full">
                          {service.service_type}
                        </span>
                        <span className={`text-xs px-2 py-1 rounded-full ${getStatusColor(service.status)}`}>
                          {service.status}
                        </span>
                      </div>
                    </div>
                    
                    <div className="flex items-center">
                      <div className="w-full bg-gray-200 rounded-full h-2.5">
                        <div 
                          className="bg-blue-600 h-2.5 rounded-full" 
                          style={{ width: `${calculateProgress(service)}%` }}
                        ></div>
                      </div>
                      <span className="ml-2 text-sm text-blue-800">{calculateProgress(service)}%</span>
                    </div>
                    
                    <p className="text-sm text-blue-600 mt-2">
                      {getServiceDetails(service)}
                    </p>
                    
                    {typeof service.metadata?.start_time === 'string' && (
                      <p className="text-xs text-gray-500 mt-2">
                        Started: {new Date(service.metadata.start_time).toLocaleString()}
                      </p>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        <div className="mb-6">
          <h2 className="text-lg font-medium text-[#0F172A] mb-4">Ecosystem Dashboard</h2>
          <div className="bg-white border border-gray-200 rounded-lg shadow-sm h-[600px] overflow-hidden">
            {loading && (
              <div className="h-full flex items-center justify-center">
                <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600"></div>
              </div>
            )}
            
            {error && (
              <div className="h-full flex items-center justify-center text-red-600">
                <p>{error}</p>
              </div>
            )}
            
            {!loading && !error && dashboardUrl && (
              <iframe
                src={dashboardUrl}
                frameBorder="0"
                width="100%"
                height="100%"
                title="Ecosystem Analytics Dashboard"
              />
            )}
          </div>
        </div>
      </main>
    </div>
  );
} 