import '@testing-library/jest-dom'
import { vi, beforeAll } from 'vitest'

// Mock Next.js Navigation
vi.mock('next/navigation', () => ({
  useRouter: () => ({
    push: vi.fn(),
    replace: vi.fn(),
    prefetch: vi.fn(),
  }),
  useSearchParams: () => ({
    get: vi.fn(),
  }),
}))

// Mock Clerk
vi.mock('@clerk/nextjs', () => ({
  useUser: () => ({
    isLoaded: true,
    isSignedIn: true,
    user: {
      id: 'user_123',
      publicMetadata: {},
      update: vi.fn(),
    },
  }),
  useAuth: () => ({
    isLoaded: true,
    userId: 'user_123',
    getToken: vi.fn(() => Promise.resolve('mock-token')),
  }),
  ClerkProvider: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}))

// Mock fetch
import createFetchMock from 'vitest-fetch-mock'
const fetchMock = createFetchMock(vi)
fetchMock.enableMocks()
