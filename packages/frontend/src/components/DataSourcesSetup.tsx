import { useApi } from '../contexts/ApiContext';
import { useState } from 'react';
import Dashboard from './Dashboard';

export default function DataSourcesSetup() {
  const { user, fetchData } = useApi();
  const [activeSetup, setActiveSetup] = useState<string | null>(null);
  const [completedSetups, setCompletedSetups] = useState<Set<string>>(new Set());
  const [showDashboard, setShowDashboard] = useState(false);
  const [contractForm, setContractForm] = useState({
    address: '',
    startBlock: '',
    abi: ''
  });
  const [githubForm, setGithubForm] = useState({
    apiKey: '',
    searchKeyword: ''
  });
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleContractSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    
    if (!user?.tenant_id) {
      setError('User tenant information not available');
      return;
    }

    // Validate contract address format
    if (!contractForm.address || !contractForm.address.startsWith('0x') || contractForm.address.length !== 42) {
      setError('Invalid contract address format. Must be a valid Ethereum address (0x...)');
      return;
    }

    // Validate start block
    if (!contractForm.startBlock) {
      setError('Start block is required');
      return;
    }
    
    const startBlock = parseInt(contractForm.startBlock);
    if (isNaN(startBlock) || startBlock <= 0) {
      setError('Start block must be a positive number');
      return;
    }

    // Validate ABI
    if (!contractForm.abi) {
      setError('Contract ABI is required');
      return;
    }

    try {
      // Validate ABI is valid JSON
      JSON.parse(contractForm.abi);
    } catch {
      setError('Invalid ABI JSON format');
      return;
    }

    setIsLoading(true);
    setError(null);

    try {
      // Call API to register contract
      await fetchData('/contracts', {
        method: 'POST',
        body: JSON.stringify({
          tenant_id: user.tenant_id,
          contract_address: contractForm.address,
          contract_abi: contractForm.abi,
          start_block: startBlock
        })
      });

      // Success - add to completed setups
      setCompletedSetups(prev => new Set(prev).add('contract'));
      setActiveSetup(null);
    } catch (err) {
      console.error('Failed to register contract:', err);
      setError(err instanceof Error ? err.message : 'Failed to register contract');
    } finally {
      setIsLoading(false);
    }
  };

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = e.target.files;
    if (files) {
      console.log('Files selected:', files);
      // Mock success
      setCompletedSetups(prev => new Set(prev).add('luma'));
      setActiveSetup(null);
    }
  };

  const handleGithubSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    console.log('GitHub setup:', githubForm);
    // Mock success
    setCompletedSetups(prev => new Set(prev).add('github'));
    setActiveSetup(null);
  };

  const handleContinueToDashboard = async () => {
    if (user?.tenant_id && completedSetups.has('contract')) {
      setIsLoading(true);
      try {
        // Start any services that aren't running yet
        await fetchData('/services/start', {
          method: 'POST',
          body: JSON.stringify({
            tenant_id: user.tenant_id
          })
        });
      } catch (err) {
        console.error('Failed to start services:', err);
        // Continue anyway - we'll show the dashboard
      } finally {
        setIsLoading(false);
      }
    }
    
    setShowDashboard(true);
  };

  if (showDashboard) {
    return <Dashboard />;
  }

  const renderSetupForm = () => {
    switch (activeSetup) {
      case 'contract':
        return (
          <form onSubmit={handleContractSubmit} className="space-y-4">
            {error && (
              <div className="bg-red-50 p-3 rounded-lg text-red-700 text-sm">
                {error}
              </div>
            )}
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Contract Address</label>
              <input
                type="text"
                value={contractForm.address}
                onChange={e => setContractForm(prev => ({ ...prev, address: e.target.value }))}
                placeholder="0x..."
                className="w-full px-4 py-2 rounded-lg border border-[#E2E8F0] focus:ring-2 focus:ring-blue-500 focus:border-transparent"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Start Block</label>
              <input
                type="number"
                value={contractForm.startBlock}
                onChange={e => setContractForm(prev => ({ ...prev, startBlock: e.target.value }))}
                placeholder="Enter block number to start indexing from"
                className="w-full px-4 py-2 rounded-lg border border-[#E2E8F0] focus:ring-2 focus:ring-blue-500 focus:border-transparent"
              />
              <p className="text-xs text-gray-500 mt-1">
                Enter the block number where contract was deployed or where you want to start indexing.
              </p>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">ABI JSON</label>
              <textarea
                value={contractForm.abi}
                onChange={e => setContractForm(prev => ({ ...prev, abi: e.target.value }))}
                placeholder="[{...}]"
                rows={4}
                className="w-full px-4 py-2 rounded-lg border border-[#E2E8F0] focus:ring-2 focus:ring-blue-500 focus:border-transparent"
              />
            </div>
            <div className="flex space-x-3">
              <button
                type="submit"
                disabled={isLoading}
                className="flex-1 py-2 px-4 rounded-lg font-medium text-white bg-[#4F46E5] hover:bg-[#4338CA] focus:ring-2 focus:ring-offset-2 focus:ring-[#4F46E5] disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {isLoading ? 'Saving...' : 'Save Contract'}
              </button>
              <button
                type="button"
                onClick={() => {
                  setActiveSetup(null);
                  setError(null);
                }}
                disabled={isLoading}
                className="py-2 px-4 rounded-lg font-medium text-gray-700 bg-gray-100 hover:bg-gray-200 disabled:opacity-50 disabled:cursor-not-allowed"
              >
                Cancel
              </button>
            </div>
          </form>
        );

      case 'luma':
        return (
          <div className="space-y-4">
            <div className="border-2 border-dashed border-gray-300 rounded-lg p-6 text-center">
              <input
                type="file"
                accept=".csv"
                multiple
                onChange={handleFileUpload}
                className="hidden"
                id="csv-upload"
              />
              <label htmlFor="csv-upload" className="cursor-pointer">
                <div className="text-gray-600">
                  <svg className="mx-auto h-12 w-12 text-gray-400" stroke="currentColor" fill="none" viewBox="0 0 48 48">
                    <path d="M28 8H12a4 4 0 00-4 4v20m32-12v8m0 0v8a4 4 0 01-4 4H12a4 4 0 01-4-4v-4m32-4l-3.172-3.172a4 4 0 00-5.656 0L28 28M8 32l9.172-9.172a4 4 0 015.656 0L28 28m0 0l4 4m4-24h8m-4-4v8m-12 4h.02" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
                  </svg>
                  <p className="mt-1">Drop your Luma attendees CSV files here or click to browse</p>
                  <p className="text-sm text-gray-500">Supports multiple files</p>
                </div>
              </label>
            </div>
            <button
              type="button"
              onClick={() => setActiveSetup(null)}
              className="w-full py-2 px-4 rounded-lg font-medium text-gray-700 bg-gray-100 hover:bg-gray-200"
            >
              Cancel
            </button>
          </div>
        );

      case 'github':
        return (
          <form onSubmit={handleGithubSubmit} className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">GitHub API Key</label>
              <input
                type="password"
                value={githubForm.apiKey}
                onChange={e => setGithubForm(prev => ({ ...prev, apiKey: e.target.value }))}
                placeholder="ghp_..."
                className="w-full px-4 py-2 rounded-lg border border-[#E2E8F0] focus:ring-2 focus:ring-blue-500 focus:border-transparent"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Search Keyword</label>
              <input
                type="text"
                value={githubForm.searchKeyword}
                onChange={e => setGithubForm(prev => ({ ...prev, searchKeyword: e.target.value }))}
                placeholder="web3, blockchain, etc."
                className="w-full px-4 py-2 rounded-lg border border-[#E2E8F0] focus:ring-2 focus:ring-blue-500 focus:border-transparent"
              />
            </div>
            <div className="flex space-x-3">
              <button
                type="submit"
                className="flex-1 py-2 px-4 rounded-lg font-medium text-white bg-[#4F46E5] hover:bg-[#4338CA] focus:ring-2 focus:ring-offset-2 focus:ring-[#4F46E5]"
              >
                Save GitHub Config
              </button>
              <button
                type="button"
                onClick={() => setActiveSetup(null)}
                className="py-2 px-4 rounded-lg font-medium text-gray-700 bg-gray-100 hover:bg-gray-200"
              >
                Cancel
              </button>
            </div>
          </form>
        );

      default:
        return (
          <div className="space-y-4">
            <button
              onClick={() => setActiveSetup('contract')}
              className={`w-full flex items-center justify-between p-4 rounded-lg border transition-colors ${
                completedSetups.has('contract')
                  ? 'border-green-500 bg-green-50'
                  : 'border-[#E2E8F0] hover:border-blue-500'
              }`}
            >
              <div className="text-left">
                <div className="flex items-center space-x-2">
                  <h3 className="font-medium text-[#0F172A]">Smart Contract</h3>
                  {completedSetups.has('contract') && (
                    <span className="text-green-600">
                      <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M5 13l4 4L19 7" />
                      </svg>
                    </span>
                  )}
                </div>
                <p className="text-[#475569] text-sm">Set up contract address, ABI, and starting block</p>
              </div>
              <span className={`${completedSetups.has('contract') ? 'text-green-600' : 'text-blue-600'}`}>
                {completedSetups.has('contract') ? 'Completed ✓' : 'Configure →'}
              </span>
            </button>

            <button
              onClick={() => setActiveSetup('luma')}
              className={`w-full flex items-center justify-between p-4 rounded-lg border transition-colors ${
                completedSetups.has('luma')
                  ? 'border-green-500 bg-green-50'
                  : 'border-[#E2E8F0] hover:border-blue-500'
              }`}
            >
              <div className="text-left">
                <div className="flex items-center space-x-2">
                  <h3 className="font-medium text-[#0F172A]">Luma Attendees</h3>
                  {completedSetups.has('luma') && (
                    <span className="text-green-600">
                      <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M5 13l4 4L19 7" />
                      </svg>
                    </span>
                  )}
                </div>
                <p className="text-[#475569] text-sm">Upload CSV exports from Luma</p>
              </div>
              <span className={`${completedSetups.has('luma') ? 'text-green-600' : 'text-blue-600'}`}>
                {completedSetups.has('luma') ? 'Completed ✓' : 'Upload →'}
              </span>
            </button>

            <button
              onClick={() => setActiveSetup('github')}
              className={`w-full flex items-center justify-between p-4 rounded-lg border transition-colors ${
                completedSetups.has('github')
                  ? 'border-green-500 bg-green-50'
                  : 'border-[#E2E8F0] hover:border-blue-500'
              }`}
            >
              <div className="text-left">
                <div className="flex items-center space-x-2">
                  <h3 className="font-medium text-[#0F172A]">GitHub Analysis</h3>
                  {completedSetups.has('github') && (
                    <span className="text-green-600">
                      <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M5 13l4 4L19 7" />
                      </svg>
                    </span>
                  )}
                </div>
                <p className="text-[#475569] text-sm">Configure GitHub API and search parameters</p>
              </div>
              <span className={`${completedSetups.has('github') ? 'text-green-600' : 'text-blue-600'}`}>
                {completedSetups.has('github') ? 'Completed ✓' : 'Configure →'}
              </span>
            </button>
          </div>
        );
    }
  };

  return (
    <div className="min-h-screen bg-white flex items-center justify-center">
      <div className="w-full max-w-md px-6">
        <div className="text-center mb-8">
          <h1 className="text-[32px] font-bold text-[#0F172A] mb-3">Setup Data Sources</h1>
          <p className="text-[#475569] text-lg">Configure your ecosystem insights</p>
        </div>

        <div className="space-y-6">
          <div className="bg-blue-50 p-4 rounded-lg">
            <h2 className="font-medium text-blue-800">Welcome to {user?.tenant_name}!</h2>
            <p className="text-blue-600 mt-1">
              Logged in as <span className="font-medium">{user?.email}</span>
            </p>
            <p className="text-blue-600 mt-1">Let's set up your data sources to start gathering insights.</p>
          </div>

          {renderSetupForm()}

          {!activeSetup && (
            <button
              onClick={handleContinueToDashboard}
              disabled={isLoading}
              className={`w-full py-3 px-4 rounded-lg font-medium text-white focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-[#4F46E5] transition-colors disabled:opacity-50 ${
                completedSetups.size > 0
                  ? 'bg-green-600 hover:bg-green-700'
                  : 'bg-[#4F46E5] hover:bg-[#4338CA]'
              }`}
            >
              {isLoading ? 'Starting indexers...' : 
                completedSetups.size > 0 ? 'Continue to Dashboard' : 'Skip to Dashboard'}
            </button>
          )}
        </div>
      </div>
    </div>
  );
} 