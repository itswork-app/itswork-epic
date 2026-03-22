import { Header } from "@/components/Header";
import { SearchBar } from "@/components/SearchBar";

export default function Home() {
  return (
    <main className="min-h-dvh bg-[#030508] relative overflow-hidden flex flex-col text-white selection:bg-blue-500/30">
      {/* High-end ambient mesh gradients */}
      <div className="absolute top-[-10%] left-1/2 -translate-x-1/2 w-[120vw] max-w-[1400px] h-[70vh] opacity-[0.15] blur-[160px] bg-linear-to-b from-blue-700 via-indigo-900 to-transparent pointer-events-none" />
      <div className="absolute bottom-[-20%] left-[-10%] w-[60vw] h-[60vh] opacity-[0.12] blur-[150px] bg-linear-to-tr from-violet-900 via-fuchsia-900 to-transparent pointer-events-none" />
      
      <Header />
      
      {/* Main Container */}
      <div className="flex-1 flex flex-col items-center justify-center p-6 sm:p-12 relative z-10 w-full max-w-5xl mx-auto mt-10">
        
        {/* Hero Section */}
        <div className="space-y-10 mb-20 text-center animate-in fade-in slide-in-from-bottom-12 duration-1000 fill-mode-both">
           
           {/* Status Badge */}
           <div className="inline-flex items-center px-4 py-2 text-xs font-mono font-medium tracking-widest uppercase border rounded-full border-blue-500/20 bg-blue-500/5 text-blue-300 shadow-[0_0_30px_rgba(59,130,246,0.1)] backdrop-blur-md">
             <span className="w-2 h-2 rounded-full bg-blue-500 mr-3 shadow-[0_0_8px_rgba(59,130,246,0.8)] animate-pulse"></span>
             V8 Engine Active
           </div>
           
           {/* Elite Typography Heading */}
           <h1 className="text-5xl sm:text-7xl md:text-[6rem] font-heading font-black tracking-tight leading-[1.05] drop-shadow-2xl">
             <span className="text-white/90">ItsWork<span className="text-blue-500">.</span></span>
             <br />
             <span className="bg-linear-to-r from-blue-400 via-indigo-400 to-violet-400 bg-clip-text text-transparent">Deep Token Intelligence</span>
           </h1>
           
           {/* Subtitle */}
           <p className="text-xl sm:text-2xl text-slate-400/90 max-w-3xl mx-auto font-sans font-light leading-relaxed tracking-wide">
             <strong className="text-white font-medium">Audit before you ape.</strong> Submit a Solana mint address for real-time AI analysis. <br className="hidden sm:block"/>
             Detect rugs, honeypots, and hidden vulnerabilities before they drain your wallet.
           </p>
        </div>
        
        {/* Search Component */}
        <div className="w-full relative z-20">
          <SearchBar />
        </div>
        
      </div>
      
      {/* Subtle Institutional Footer */}
      <footer className="absolute bottom-6 w-full text-center text-xs font-mono text-white/30 tracking-widest uppercase z-10 pointer-events-none">
        ItsWork.app Institutional Grade Security
      </footer>
    </main>
  );
}
