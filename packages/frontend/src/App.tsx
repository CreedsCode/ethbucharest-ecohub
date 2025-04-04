import { useState } from 'react'
import { useApi } from './contexts/ApiContext'
import DataSourcesSetup from './components/DataSourcesSetup'

function App() {
  const { register, loading, error, isAuthenticated } = useApi()
  const [formData, setFormData] = useState({
    email: '',
    password: '',
    confirmPassword: '',
    tenantName: ''
  })

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const { name, value } = e.target
    setFormData(prev => ({
      ...prev,
      [name]: value
    }))
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (formData.password !== formData.confirmPassword) {
      alert('Passwords do not match!')
      return
    }
    try {
      await register(formData.email, formData.password, formData.tenantName)
    } catch (err) {
      console.error('Registration failed:', err)
    }
  }

  if (isAuthenticated) {
    return <DataSourcesSetup />
  }

  return (
    <div className="min-h-screen bg-white flex items-center justify-center">
      <div className="w-full max-w-md px-6">
        <div className="text-center mb-8">
          <h1 className="text-[32px] font-bold text-[#0F172A] mb-3">Welcome to Web3 Insights</h1>
          <p className="text-[#475569] text-lg">Start your ecosystem insights journey</p>
        </div>

        <form onSubmit={handleSubmit} className="space-y-6">
          <div>
            <label htmlFor="tenantName" className="block text-[#475569] text-base mb-2">
              Organization Name
            </label>
            <input
              id="tenantName"
              name="tenantName"
              type="text"
              required
              className="w-full px-4 py-3 rounded-lg border border-[#E2E8F0] text-[#0F172A] placeholder-[#94A3B8] focus:outline-none focus:ring-2 focus:ring-[#3B82F6] focus:border-transparent"
              value={formData.tenantName}
              onChange={handleChange}
              placeholder="Your organization name"
            />
          </div>

          <div>
            <label htmlFor="email" className="block text-[#475569] text-base mb-2">
              Email address
            </label>
            <input
              id="email"
              name="email"
              type="email"
              required
              className="w-full px-4 py-3 rounded-lg border border-[#E2E8F0] text-[#0F172A] placeholder-[#94A3B8] focus:outline-none focus:ring-2 focus:ring-[#3B82F6] focus:border-transparent"
              value={formData.email}
              onChange={handleChange}
              placeholder="you@company.com"
            />
          </div>
          
          <div>
            <label htmlFor="password" className="block text-[#475569] text-base mb-2">
              Password
            </label>
            <input
              id="password"
              name="password"
              type="password"
              required
              className="w-full px-4 py-3 rounded-lg border border-[#E2E8F0] text-[#0F172A] placeholder-[#94A3B8] focus:outline-none focus:ring-2 focus:ring-[#3B82F6] focus:border-transparent"
              value={formData.password}
              onChange={handleChange}
              placeholder="••••••••"
            />
          </div>
          
          <div>
            <label htmlFor="confirmPassword" className="block text-[#475569] text-base mb-2">
              Confirm Password
            </label>
            <input
              id="confirmPassword"
              name="confirmPassword"
              type="password"
              required
              className="w-full px-4 py-3 rounded-lg border border-[#E2E8F0] text-[#0F172A] placeholder-[#94A3B8] focus:outline-none focus:ring-2 focus:ring-[#3B82F6] focus:border-transparent"
              value={formData.confirmPassword}
              onChange={handleChange}
              placeholder="••••••••"
            />
          </div>

          <button
            type="submit"
            disabled={loading}
            className="w-full py-3 px-4 rounded-lg font-medium text-white bg-[#4F46E5] hover:bg-[#4338CA] focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-[#4F46E5] disabled:bg-[#94A3B8] disabled:cursor-not-allowed transition-colors"
          >
            {loading ? 'Creating account...' : 'Join the ecosystem'}
          </button>
        </form>

        {error && (
          <div className="mt-4 text-sm text-red-600">
            {error}
          </div>
        )}
      </div>
    </div>
  )
}

export default App
