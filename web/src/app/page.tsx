import { Header } from "@/components/Header";
import { SearchBar } from "@/components/SearchBar";

export default function Home() {
  return (
    <main className="min-h-dvh bg-[#020205] relative overflow-hidden flex flex-col text-white font-sans selection:bg-blue-500/30">
      {/* Dynamic ambient backgrounds */}
      <div className="absolute top-0 left-1/2 -translate-x-1/2 w-[80vw] max-w-[1000px] h-[50vh] opacity-30 blur-[140px] bg-linear-to-b from-blue-900 via-indigo-900 to-transparent pointer-events-none" />
      <div className="absolute bottom-0 left-0 w-[50vw] h-[50vh] opacity-20 blur-[130px] bg-linear-to-tr from-purple-900 to-transparent pointer-events-none" />
      
      <Header />
      
      <div className="flex-1 flex flex-col items-center justify-center p-6 sm:p-12 relative z-10 w-full max-w-6xl mx-auto text-center mt-[-5vh]">
        
        <div className="space-y-8 mb-16 animate-in fade-in slide-in-from-bottom-12 duration-1000 fill-mode-both">
           <div className="inline-flex items-center px-4 py-1.5 text-xs font-bold tracking-widest uppercase border rounded-full border-blue-500/30 bg-blue-500/10 text-blue-300 shadow-[0_0_15px_rgba(59,130,246,0.2)]">
             <span className="w-2 h-2 rounded-full bg-blue-400 mr-2 animate-pulse"></span>
             V8 Engine Active
           </div>
           
           <h1 className="text-6xl sm:text-8xl font-black tracking-tighter leading-[1.1]">
             Industrial <span className="bg-linear-to-br from-blue-400 via-indigo-300 to-purple-400 bg-clip-text text-transparent">Intelligence</span>
             <br /> for Solana
           </h1>
           
           <p className="text-xl sm:text-2xl text-gray-400 max-w-3xl mx-auto font-medium leading-relaxed">
             Submit a token mint address for real-time deep AI auditing. 
             Detect rugs, hidden vulnerabilities, and verify trust in milliseconds.
           </p>
        </div>
        
        <div className="w-full relative z-20">
          <SearchBar />
        </div>
        
      </div>
    </main>
  );
}
