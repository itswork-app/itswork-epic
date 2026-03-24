"use client";

import { useUser } from "@clerk/nextjs";
import { 
  TrendingUp, 
  History, 
  Zap, 
  AlertCircle,
  QrCode,
  ArrowRight
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";

export default function TraderDashboard() {
  const { user, isLoaded } = useUser();
  const scansUsed = 1;
  const totalScans = 3;
  
  if (!isLoaded) return null;

  return (
    <div className="min-h-screen bg-[#030508] text-white font-sans selection:bg-blue-500/30">
      {/* Top Navigation Bar */}
      <nav className="h-20 border-b border-white/5 bg-[#05070a]/80 backdrop-blur-xl px-8 flex items-center justify-between sticky top-0 z-50">
        <div className="flex items-center space-x-12">
          <h1 className="text-xl font-heading font-black tracking-tighter cursor-pointer hover:opacity-80 transition-opacity">
            ITS<span className="text-blue-500">WORK</span>.APP
          </h1>
          <div className="hidden md:flex items-center space-x-8 text-sm font-medium text-slate-400">
            <span className="text-white border-b-2 border-blue-500 pb-7 mt-7">Terminal</span>
            <span className="hover:text-white transition-colors cursor-pointer">History</span>
            <span className="hover:text-white transition-colors cursor-pointer">Community Signals</span>
          </div>
        </div>
        <div className="flex items-center space-x-6">
          <div className="flex flex-col items-end">
            <span className="text-[10px] font-mono text-slate-500 uppercase tracking-widest">Global Sec Status</span>
            <span className="text-xs font-mono text-emerald-400">OPTIMIZED</span>
          </div>
          <div className="w-10 h-10 rounded-full bg-blue-500/10 border border-blue-500/20" />
        </div>
      </nav>

      <main className="p-6 md:p-12 max-w-6xl mx-auto space-y-12">
        {/* Welcome & Quota Tracker */}
        <header className="flex flex-col md:flex-row md:items-end justify-between gap-8 py-4">
          <div className="space-y-3">
             <h2 className="text-3xl md:text-5xl font-heading font-bold tracking-tight">
               Welcome back, <span className="text-blue-400 truncate">{user?.firstName || "Trader"}</span>
             </h2>
             <p className="text-slate-400 font-sans max-w-md">
               Your institutional-grade intelligence portal is synchronized with itswork-ingestor v8.
             </p>
          </div>
          
          <div className="w-full md:w-80 p-6 rounded-2xl bg-white/5 border border-white/10 space-y-4">
            <div className="flex justify-between items-center text-xs font-mono uppercase tracking-widest">
              <span className="text-slate-500">Daily Free Scans</span>
              <span className="text-blue-400">{totalScans - scansUsed} Remaining</span>
            </div>
            <Progress value={(scansUsed / totalScans) * 100} className="h-2 bg-white/5" />
            <p className="text-[10px] text-slate-500 font-sans text-center">Resets in 14 hours 22 minutes</p>
          </div>
        </header>

        <div className="grid grid-cols-1 lg:grid-cols-3 gap-12">
          {/* Main Analysis Area */}
          <section className="lg:col-span-2 space-y-8">
            <h3 className="text-xl font-bold flex items-center space-x-3">
              <Zap className="w-5 h-5 text-blue-400 fill-blue-400/20" />
              <span>Active Target Intelligence</span>
            </h3>
            
            <div className="p-8 md:p-10 rounded-3xl border border-white/10 bg-[#090b14]/60 backdrop-blur-xl space-y-10 shadow-2xl relative overflow-hidden group">
               <div className="absolute top-0 right-0 p-6">
                 <div className="flex items-center space-x-2 px-3 py-1 rounded-full bg-rose-500/10 border border-rose-500/20 text-rose-500 text-[10px] font-mono font-bold animate-pulse">
                   <AlertCircle className="w-3 h-3" />
                   <span>HIGH RISK DETECTED</span>
                 </div>
               </div>

               <div className="flex flex-col md:flex-row items-center gap-10">
                 <div className="relative">
                   <div className="absolute inset-0 bg-rose-500/30 blur-3xl opacity-40 animate-pulse" />
                   <div className="relative w-32 h-32 md:w-40 md:h-40 rounded-full border-4 border-rose-500/30 flex items-center justify-center bg-black/40 backdrop-blur-md">
                      <div className="text-center">
                        <span className="text-4xl md:text-5xl font-mono font-black text-white">12</span>
                        <div className="text-[10px] font-mono text-slate-500 uppercase font-bold tracking-tighter">AI Score</div>
                      </div>
                   </div>
                 </div>
                 
                 <div className="flex-1 text-center md:text-left space-y-4">
                   <h4 className="text-4xl md:text-6xl font-heading font-black tracking-widest text-rose-500 uppercase italic">DANGER</h4>
                   <p className="text-slate-400 font-sans leading-relaxed text-sm md:text-base border-l-2 border-white/10 pl-6 py-2">
                     <strong className="text-white">Narrative Verdict:</strong> Kreator ini terdeteksi memiliki riwayat &quot;Serial Rugger&quot;. Tim intelijen kami menemukan keterkaitan on-chain dengan koin <span className="text-blue-400">$MOKONDO</span> yang di-drain dalam 4 menit setelah peluncuran. 
                   </p>
                 </div>
               </div>

               <div className="grid grid-cols-2 md:grid-cols-3 gap-4 pt-4">
                 {[
                   { label: "Insider Risk", val: "Critical", color: "text-rose-500" },
                   { label: "Creator Rep", val: "Blacklisted", color: "text-rose-500" },
                   { label: "Holders", val: "Bot Clusters", color: "text-amber-500" }
                 ].map((stat, i) => (
                   <div key={i} className="p-4 rounded-xl bg-white/5 border border-white/5">
                     <p className="text-[10px] font-mono text-slate-500 uppercase mb-1">{stat.label}</p>
                     <p className={`font-bold text-sm ${stat.color}`}>{stat.val}</p>
                   </div>
                 ))}
               </div>
            </div>
          </section>

          {/* Side Actions / Billing Bridge */}
          <aside className="space-y-8">
            <h3 className="text-xl font-bold flex items-center space-x-3">
              <History className="w-5 h-5 text-blue-400" />
              <span>Operations</span>
            </h3>
            
            <div className="space-y-6">
              <div className="p-6 rounded-3xl border border-white/10 bg-linear-to-b from-blue-600/10 to-transparent space-y-6">
                <div className="space-y-2">
                  <h4 className="font-bold">Unlock Full Intelligence</h4>
                  <p className="text-xs text-slate-400 leading-relaxed font-sans">
                    Scan limits reached? Purchase institutional access to continue using the deep-audit engine.
                  </p>
                </div>
                
                <div className="p-4 rounded-2xl bg-black/40 border border-white/10 flex items-center justify-center flex-col space-y-4">
                   <div className="w-32 h-32 bg-white rounded-xl p-2">
                      <QrCode className="w-full h-full text-black" />
                   </div>
                   <div className="text-center space-y-1">
                      <p className="text-[10px] font-mono text-slate-500 uppercase">Solana Pay Gateway</p>
                      <p className="text-sm font-bold">0.05 SOL / Scan</p>
                   </div>
                </div>

                <Button className="w-full h-12 rounded-xl bg-white text-black hover:bg-slate-200 font-bold transition-all hover:scale-[1.02]">
                  Upgrade to Pro Plan
                </Button>
              </div>

              <div className="p-6 rounded-3xl border border-white/10 bg-white/5 flex items-center justify-between group cursor-pointer hover:border-blue-500/30 transition-all">
                <div className="flex items-center space-x-4">
                   <div className="w-10 h-10 rounded-full bg-blue-500/10 flex items-center justify-center text-blue-400">
                      <TrendingUp className="w-5 h-5" />
                   </div>
                   <span className="text-sm font-bold">View Global Trends</span>
                </div>
                <ArrowRight className="w-4 h-4 text-slate-600 group-hover:text-white group-hover:translate-x-1 transition-all" />
              </div>
            </div>
          </aside>
        </div>
      </main>

      <footer className="footer-institutional py-12 border-t border-white/5 text-center mt-20">
        <p className="text-xs font-mono text-white/20 tracking-[0.4em] uppercase">ItsWork Terminal Institutional Framework</p>
      </footer>
    </div>
  );
}
