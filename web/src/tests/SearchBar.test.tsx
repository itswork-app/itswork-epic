import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { SearchBar } from '@/components/SearchBar'
import { vi, describe, it, expect } from 'vitest'

// Mock UI components that might cause issues in JSDOM
vi.mock('@/components/ui/button', () => ({
  Button: ({ children, ...props }: { children: React.ReactNode }) => <button {...props}>{children}</button>
}))

vi.mock('@/components/ui/input', () => ({
  Input: (props: React.InputHTMLAttributes<HTMLInputElement>) => <input {...props} />
}))

describe('SearchBar', () => {
  it('renders search input', () => {
    render(<SearchBar />)
    expect(screen.getByPlaceholderText(/Enter Solana mint/i)).toBeInTheDocument()
  })

  it('shows intelligent blur for guest teaser results', async () => {
    // Mock successful teaser response
    (global.fetch as any).mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        mint: '7vfCXTUX...',
        score: 85,
        verdict: 'SAFE',
        teaser: true,
        reason: 'Clean history confirmed.',
        creator_reputation: 'TRUSTED',
        insider_risk: 'Low'
      })
    })

    render(<SearchBar />)
    const input = screen.getByPlaceholderText(/Enter Solana mint/i)
    fireEvent.change(input, { target: { value: '7vfCXTUX...' } })
    fireEvent.click(screen.getByRole('button', { name: /Run Analysis/i }))

    await waitFor(() => {
      expect(screen.getByText(/SAFE/i)).toBeInTheDocument()
    })

    // Check for blur CTA
    expect(screen.getByText(/Login to Unlock Full Intelligence/i)).toBeInTheDocument()
  })
})
