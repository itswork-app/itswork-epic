"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useUser, useAuth } from "@clerk/nextjs";
import { LineChart, Code2, ArrowRight, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";

export default function OnboardingPage() {
  const { isLoaded: userLoaded } = useUser();
  const { getToken } = useAuth();
  const router = useRouter();
  const [loading, setLoading] = useState<string | null>(null);

  const selectRole = async (role: "trader" | "developer") => {
    setLoading(role);
    try {
      const token = await getToken();
      const res = await fetch("/api/v1/user/role", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ role }),
      });

      if (res.ok) {
        router.push(role === "trader" ? "/dashboard/trader" : "/dashboard/developer");
      }
    } catch (err) {
      console.error("Role selection failed:", err);
    } finally {
      setLoading(null);
    }
  };

  if (!userLoaded) return null;

  return (
    <main className="min-h-screen bg-[#030508] relative overflow-hidden flex flex-col items-center justify-center p-6 text-white selection:bg-blue-500/30">
      {/* Ambient backgrounds */}
      <div className="absolute top-[-20%] left-[-10%] w-[60vw] h-[60vh] opacity-[0.1] blur-[120px] bg-blue-600 pointer-events-none" />
      <div className="absolute bottom-[-10%] right-[-10%] w-[50vw] h-[50vh] opacity-[0.08] blur-[120px] bg-indigo-600 pointer-events-none" />

      <div className="w-full max-w-4xl relative z-10 space-y-16 text-center">
        <div className="space-y-4 animate-in fade-in slide-in-from-bottom-8 duration-700">
          <div className="inline-flex items-center px-3 py-1 text-[10px] font-mono font-medium tracking-[0.2em] uppercase border rounded-full border-blue-500/30 bg-blue-500/10 text-blue-400">
            Path Selection Required
          </div>
          <h1 className="text-4xl sm:text-6xl font-heading font-black tracking-tight leading-tight">
            Choose Your <span className="bg-linear-to-r from-blue-400 to-indigo-400 bg-clip-text text-transparent">War Path</span>
          </h1>
          <p className="text-lg text-slate-400 max-w-2xl mx-auto font-sans font-light">
            ItsWork.app adapts its intelligence engine to your specific field of engagement. Select your primary role to configure your terminal.
          </p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-8 auto-rows-fr">
          {/* Trader Path */}
          <div className="group relative">
            <div className="absolute inset-0 bg-blue-500/20 blur-2xl opacity-0 group-hover:opacity-100 transition-opacity duration-500 pointer-events-none" />
            <div className="relative h-full p-8 rounded-3xl border border-white/10 bg-[#090b14]/60 backdrop-blur-xl flex flex-col space-y-8 transition-all duration-300 hover:border-blue-500/50 hover:translate-y-[-4px]">
              <div className="w-16 h-16 rounded-2xl bg-blue-500/10 border border-blue-500/20 flex items-center justify-center text-blue-400">
                <LineChart className="w-8 h-8" />
              </div>
              <div className="flex-1 space-y-4 text-left">
                <h2 className="text-2xl font-bold tracking-tight">Human Trader</h2>
                <ul className="space-y-3 text-sm text-slate-400 font-sans">
                  <li className="flex items-center space-x-3">
                    <div className="w-1 h-1 rounded-full bg-blue-500" />
                    <span>Narrative AI Fraud Detection</span>
                  </li>
                  <li className="flex items-center space-x-3">
                    <div className="w-1 h-1 rounded-full bg-blue-500" />
                    <span><strong>3 Free Manual Scans</strong> per day</span>
                  </li>
                  <li className="flex items-center space-x-3">
                    <div className="w-1 h-1 rounded-full bg-blue-500" />
                    <span>Interactive Solana Pay Terminal</span>
                  </li>
                </ul>
              </div>
              <Button 
                onClick={() => selectRole("trader")}
                disabled={loading !== null}
                className="w-full h-14 rounded-xl bg-blue-600 hover:bg-blue-500 text-white font-bold group/btn"
              >
                {loading === "trader" ? <Loader2 className="w-5 h-5 animate-spin" /> : (
                  <>
                    Enlist as Trader 
                    <ArrowRight className="ml-2 w-4 h-4 group-hover/btn:translate-x-1 transition-transform" />
                  </>
                )}
              </Button>
            </div>
          </div>

          {/* Developer Path */}
          <div className="group relative">
            <div className="absolute inset-0 bg-indigo-500/20 blur-2xl opacity-0 group-hover:opacity-100 transition-opacity duration-500 pointer-events-none" />
            <div className="relative h-full p-8 rounded-3xl border border-white/10 bg-[#090b14]/60 backdrop-blur-xl flex flex-col space-y-8 transition-all duration-300 hover:border-indigo-500/50 hover:translate-y-[-4px]">
              <div className="w-16 h-16 rounded-2xl bg-indigo-500/10 border border-indigo-500/20 flex items-center justify-center text-indigo-400">
                <Code2 className="w-8 h-8" />
              </div>
              <div className="flex-1 space-y-4 text-left">
                <h2 className="text-2xl font-bold tracking-tight">Bot Developer</h2>
                <ul className="space-y-3 text-sm text-slate-400 font-sans">
                  <li className="flex items-center space-x-3">
                    <div className="w-1 h-1 rounded-full bg-indigo-500" />
                    <span>High-Speed gRPC & REST Access</span>
                  </li>
                  <li className="flex items-center space-x-3">
                    <div className="w-1 h-1 rounded-full bg-indigo-500" />
                    <span><strong>10 Free API Calls</strong> per day</span>
                  </li>
                  <li className="flex items-center space-x-3">
                    <div className="w-1 h-1 rounded-full bg-indigo-500" />
                    <span>Institutional Developer Console</span>
                  </li>
                </ul>
              </div>
              <Button 
                onClick={() => selectRole("developer")}
                disabled={loading !== null}
                className="w-full h-14 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white font-bold group/btn"
              >
                {loading === "developer" ? <Loader2 className="w-5 h-5 animate-spin" /> : (
                  <>
                    Deploy as Developer 
                    <ArrowRight className="ml-2 w-4 h-4 group-hover/btn:translate-x-1 transition-transform" />
                  </>
                )}
              </Button>
            </div>
          </div>
        </div>

        <footer className="pt-8 text-xs font-mono text-white/20 tracking-[0.3em] uppercase">
          ItsWork Institutional Grade Identity Verification
        </footer>
      </div>
    </main>
  );
}
