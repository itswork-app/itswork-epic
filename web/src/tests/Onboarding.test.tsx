import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import OnboardingPage from '@/app/onboarding/page'
import { vi, describe, it, expect } from 'vitest'

vi.mock('@/components/ui/button', () => ({
  Button: ({ children, ...props }: any) => <button {...props}>{children}</button>
}))

describe('OnboardingPage', () => {
  it('renders role selection options', () => {
    render(<OnboardingPage />)
    expect(screen.getByText((content) => content.includes('Choose Your'))).toBeInTheDocument()
    expect(screen.getByText(/Enlist as Trader/i)).toBeInTheDocument()
    expect(screen.getByText(/Deploy as Developer/i)).toBeInTheDocument()
  })

  it('navigates to trader dashboard after selection', async () => {
    const fetchSpy = vi.spyOn(global, 'fetch').mockImplementation(() => 
      Promise.resolve({ ok: true } as Response)
    )
    
    render(<OnboardingPage />)
    
    const traderButton = screen.getByText(/Enlist as Trader/i)
    fireEvent.click(traderButton)

    await waitFor(() => {
      expect(fetchSpy).toHaveBeenCalledWith('/api/v1/user/role', expect.any(Object))
    }, { timeout: 3000 })
  })
})
