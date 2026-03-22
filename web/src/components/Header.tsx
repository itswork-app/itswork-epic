import { SignInButton, UserButton } from '@clerk/nextjs'
import { auth } from '@clerk/nextjs/server'
import Link from 'next/link'

export async function Header() {
  const { userId } = await auth();

  return (
    <header className="flex items-center justify-between p-4 border-b border-white/5 bg-black/40 backdrop-blur-xl sticky top-0 z-50">
      <div className="flex items-center space-x-2">
        <Link href="/">
          <span className="text-2xl font-black tracking-tighter bg-linear-to-r from-blue-400 to-purple-500 bg-clip-text text-transparent">ItsWork.</span>
        </Link>
      </div>
      <div>
        {!userId ? (
          <SignInButton mode="modal">
            <button className="px-5 py-2 text-sm font-semibold text-white transition-all border rounded-full border-white/10 bg-white/5 hover:bg-white/10 hover:shadow-[0_0_15px_rgba(255,255,255,0.1)]">Sign In</button>
          </SignInButton>
        ) : (
          <UserButton appearance={{ elements: { userButtonAvatarBox: "w-10 h-10 border border-white/20" } }} />
        )}
      </div>
    </header>
  )
}

