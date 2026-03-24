"use client";

import { useState } from "react";
import { useUser } from "@clerk/nextjs";
import { 
  Key, 
  Copy, 
  RotateCcw, 
  BarChart3, 
  Terminal, 
  ShieldCheck, 
  ExternalLink,
  Zap,
  Info
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

export default function DeveloperDashboard() {
  const { isLoaded } = useUser();
  const [apiKey] = useState("itswork_live_7k82jr91mnxv3p4l0q");
  
  if (!isLoaded) return null;

  return (
    <div className="min-h-screen bg-[#030508] text-white font-sans selection:bg-indigo-500/30">
      {/* Sidebar Navigation */}
      <aside className="fixed left-0 top-0 h-full w-64 border-r border-white/5 bg-[#05070a] z-50 hidden lg:block">
        <div className="p-8 border-b border-white/5">
          <h1 className="text-xl font-heading font-black tracking-tighter">
            ITS<span className="text-indigo-500">WORK</span>.DEV
          </h1>
        </div>
        <nav className="p-4 space-y-2 mt-4">
          <div className="flex items-center space-x-3 px-4 py-3 rounded-xl bg-indigo-500/10 text-indigo-400 border border-indigo-500/20">
            <Terminal className="w-5 h-5" />
            <span className="font-semibold text-sm">Console</span>
          </div>
          <div className="flex items-center space-x-3 px-4 py-3 rounded-xl text-slate-500 hover:bg-white/5 transition-all">
            <Key className="w-5 h-5" />
            <span className="font-semibold text-sm">Credentials</span>
          </div>
          <div className="flex items-center space-x-3 px-4 py-3 rounded-xl text-slate-500 hover:bg-white/5 transition-all">
            <BarChart3 className="w-5 h-5" />
            <span className="font-semibold text-sm">Analytics</span>
          </div>
        </nav>
      </aside>

      <main className="lg:ml-64 p-8 lg:p-12 max-w-7xl mx-auto space-y-12">
        {/* Header */}
        <header className="flex flex-col md:flex-row md:items-center justify-between gap-6">
          <div className="space-y-2">
            <div className="flex items-center space-x-3 text-indigo-400 font-mono text-xs uppercase tracking-[0.3em]">
              <Zap className="w-3 h-3 fill-current" />
              Developer Environment
            </div>
            <h2 className="text-4xl font-heading font-bold tracking-tight">System Console</h2>
          </div>
          <div className="flex items-center space-x-4">
            <div className="px-4 py-2 rounded-lg bg-emerald-500/10 border border-emerald-500/20 text-emerald-400 text-xs font-mono">
              API STATUS: OPERATIONAL
            </div>
          </div>
        </header>

        {/* API Key Center */}
        <section className="space-y-6">
          <div className="flex items-center justify-between border-b border-white/5 pb-4">
            <h3 className="text-xl font-bold flex items-center space-x-3">
              <Key className="w-5 h-5 text-indigo-400" />
              <span>Authentication Gateway</span>
            </h3>
          </div>
          
          <div className="p-8 rounded-3xl border border-white/10 bg-[#090b14]/60 backdrop-blur-xl space-y-8 shadow-2xl">
            <div className="flex flex-col space-y-4">
              <label className="text-xs font-mono text-slate-500 uppercase tracking-widest">Active X-API-KEY</label>
              <div className="flex flex-col sm:flex-row gap-3">
                <div className="relative flex-1 group">
                   <div className="absolute inset-y-0 left-0 pl-4 flex items-center pointer-events-none">
                     <ShieldCheck className="w-4 h-4 text-emerald-500" />
                   </div>
                   <Input 
                     readOnly 
                     value={apiKey}
                     className="bg-black/40 border-white/10 h-14 pl-12 font-mono text-indigo-300 focus:ring-indigo-500/50"
                   />
                </div>
                <div className="flex gap-2">
                  <Button variant="outline" className="h-14 px-6 border-white/10 hover:bg-white/5 text-slate-300">
                    <Copy className="w-4 h-4 mr-2" /> Copy
                  </Button>
                  <Button variant="outline" className="h-14 px-6 border-white/10 hover:bg-red-500/10 hover:text-red-400 hover:border-red-500/30 text-slate-300">
                    <RotateCcw className="w-4 h-4 mr-2" /> Revoke
                  </Button>
                </div>
              </div>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-3 gap-6 pt-4">
              <div className="p-6 rounded-2xl bg-white/5 border border-white/5 space-y-2">
                <p className="text-xs font-mono text-slate-500 uppercase tracking-tighter">Current Plan</p>
                <div className="text-lg font-bold">Bot Developer Free</div>
              </div>
              <div className="p-6 rounded-2xl bg-white/5 border border-white/5 space-y-2">
                <p className="text-xs font-mono text-slate-500 uppercase tracking-tighter">Quota Remaining</p>
                <div className="text-lg font-bold">10 / 10 Requests</div>
              </div>
              <div className="p-6 rounded-2xl bg-white/5 border border-white/5 space-y-2">
                <p className="text-xs font-mono text-slate-500 uppercase tracking-tighter">Rate Limit</p>
                <div className="text-lg font-bold">5 REQ/SEC</div>
              </div>
            </div>
          </div>
        </section>

        <div className="grid grid-cols-1 lg:grid-cols-2 gap-12">
          {/* Usage Stats (Mock) */}
          <section className="space-y-6">
            <h3 className="text-xl font-bold flex items-center space-x-3">
              <BarChart3 className="w-5 h-5 text-indigo-400" />
              <span>Network Utilization</span>
            </h3>
            <div className="h-64 rounded-3xl border border-white/10 bg-[#090b14]/60 backdrop-blur-xl flex items-center justify-center relative overflow-hidden group">
               <div className="absolute inset-0 bg-linear-to-tr from-indigo-500/5 to-transparent pointer-events-none" />
               <div className="flex flex-col items-center space-y-4">
                 <div className="flex space-x-1 items-end">
                   {[40, 70, 45, 90, 65, 80, 50, 60, 30, 85].map((h, i) => (
                     <div 
                       key={i} 
                       className="w-4 bg-indigo-500/20 rounded-t-sm transition-all duration-500 group-hover:bg-indigo-500/40" 
                       style={{ height: `${h}px` }}
                      />
                   ))}
                 </div>
                 <p className="text-xs font-mono text-slate-500 uppercase tracking-widest">Real-time usage metrics coming soon</p>
               </div>
            </div>
          </section>

          {/* Sandbox / Docs */}
          <section className="space-y-6">
            <h3 className="text-xl font-bold flex items-center space-x-3">
              <Terminal className="w-5 h-5 text-indigo-400" />
              <span>Interactive Sandbox</span>
            </h3>
            <div className="p-8 rounded-3xl border border-white/10 bg-black/60 font-mono text-sm h-64 overflow-hidden relative group">
               <div className="text-indigo-400 mb-2">$ curl -X GET &quot;https://api.itswork.app/v1/sniper/verdict/7vfC...&quot; \</div>
               <div className="text-indigo-400 mb-4">  -H &quot;X-API-KEY: {apiKey.substring(0, 12)}...&quot;</div>
               <div className="text-slate-500">{`{`}</div>
               <div className="text-slate-400 ml-4">&quot;mint&quot;: &quot;7vfCXTUX...&quot;,</div>
               <div className="text-slate-400 ml-4">&quot;score&quot;: 88,</div>
               <div className="text-emerald-400 ml-4">&quot;verdict&quot;: &quot;SAFE&quot;,</div>
               <div className="text-slate-500">{`}`}</div>
               
               <div className="absolute bottom-6 right-6">
                 <Button className="bg-indigo-600 hover:bg-indigo-500 shadow-lg shadow-indigo-600/20">
                   Execute Request <ExternalLink className="ml-2 w-4 h-4" />
                 </Button>
               </div>
            </div>
          </section>
        </div>

        {/* Note Footer */}
        <div className="flex items-start space-x-4 p-6 rounded-2xl bg-white/5 border border-white/5">
           <Info className="w-5 h-5 text-slate-400 mt-1 shrink-0" />
           <p className="text-sm text-slate-400 font-sans leading-relaxed">
             <strong>Bot Developer Policy:</strong> Free tier accounts are limited to 10 API calls per 24-hour cycle. Commercial use and higher limits require an institutional Pro/Ultra/Enterprise license. API Multiplier (3x) is applied for developer rate parity.
           </p>
        </div>
      </main>

      <footer className="footer-institutional py-12 border-t border-white/5 text-center">
        <p className="text-xs font-mono text-white/20 tracking-[0.4em] uppercase">ItsWork Terminal Institutional Framework</p>
      </footer>
    </div>
  );
}
