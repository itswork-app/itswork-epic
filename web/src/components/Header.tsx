import { SignInButton, UserButton } from '@clerk/nextjs'
import { auth } from '@clerk/nextjs/server'
import Link from 'next/link'

export async function Header() {
  const { userId } = await auth();

  return (
    <header className="fixed top-0 inset-x-0 h-20 flex items-center justify-between px-6 sm:px-10 border-b border-white/4 bg-black/30 backdrop-blur-2xl z-50 transition-colors">
      <div className="flex items-center space-x-3">
        <div className="w-8 h-8 rounded-lg bg-linear-to-tr from-blue-600 to-indigo-500 shadow-[0_0_20px_rgba(59,130,246,0.5)] flex items-center justify-center">
          <svg className="w-4 h-4 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z" />
          </svg>
        </div>
        <Link href="/">
          <span className="text-2xl font-heading font-black tracking-tight text-white hover:opacity-80 transition-opacity">
            ItsWork<span className="text-blue-500">.</span>
          </span>
        </Link>
      </div>
      <div>
        {!userId ? (
          <SignInButton mode="modal" forceRedirectUrl="/onboarding">
            <button className="px-6 py-2.5 text-sm font-medium text-white transition-all duration-300 border rounded-full border-white/10 bg-white/3 hover:bg-white/8 hover:border-white/20 hover:shadow-[0_0_30px_rgba(255,255,255,0.05)] font-sans">
              Connect Identity
            </button>
          </SignInButton>
        ) : (
          <div className="p-1 rounded-full border border-white/10 bg-white/5 hover:border-blue-500/50 transition-colors shadow-inner">
            <UserButton appearance={{ elements: { userButtonAvatarBox: "w-9 h-9 border border-white/20 rounded-full" } }} />
          </div>
        )}
      </div>
    </header>
  )
}
