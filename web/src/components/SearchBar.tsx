"use client";

import { useState } from "react";
import { useAuth } from "@clerk/nextjs";
import { Search, ShieldAlert, ShieldCheck, Loader2 } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";

export function SearchBar() {
  const [address, setAddress] = useState("");
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<{ 
    score: number; 
    verdict: string; 
    reason?: string;
    creator_reputation?: string;
    insider_risk?: string;
    teaser?: boolean 
  } | null>(null);
  const [error, setError] = useState("");
  const { getToken, isSignedIn } = useAuth();

  const handleSearch = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!address.trim()) return;

    setLoading(true);
    setResult(null);
    setError("");

    try {
      const isTeaser = !isSignedIn;
      const token = isSignedIn ? await getToken() : null;
      
      const res = await fetch(`/api/v1/token/${address.trim()}${isTeaser ? "?teaser=true" : ""}`, {
        headers: token ? { "Authorization": `Bearer ${token}` } : {}
      });

      if (!res.ok) {
        if (res.status === 403) throw new Error("Access Denied: Please upgrade your plan to scan this token.");
        if (res.status === 429) throw new Error("Rate Limit Exceeded: Slow down, agent.");
        throw new Error("Failed to fetch token intelligence");
      }
      const data = await res.json();
      setResult({
        score: data.score || 0,
        verdict: data.verdict || "UNKNOWN",
        reason: data.reason,
        creator_reputation: data.creator_reputation,
        insider_risk: data.insider_risk,
        teaser: data.teaser || false,
      });
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : "ItsWork Intelligence Service is unreachable. Check your connection.";
      setError(errorMessage);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="w-full max-w-3xl mx-auto space-y-10 animate-in fade-in slide-in-from-bottom-8 duration-1000 delay-200 fill-mode-both">
      {/* Massive Sleek Input Form */}
      <form onSubmit={handleSearch} className="relative flex items-center w-full group">
        
        <div className="absolute inset-y-0 left-0 flex items-center pl-6 pointer-events-none transition-transform duration-500 group-focus-within:scale-110">
          <Search className="w-6 h-6 text-slate-500 transition-colors duration-500 group-focus-within:text-blue-400" />
        </div>
        
        <Input
          type="text"
          placeholder="Enter Solana mint (e.g. 7vfCXTUX...)"
          className="w-full h-20 pl-16 pr-44 text-xl sm:text-2xl bg-[#090b14]/80 backdrop-blur-3xl border-white/10 text-slate-200 placeholder-slate-600 rounded-2xl focus:ring-1 focus:ring-blue-500/50 focus:border-blue-500/60 transition-all duration-500 font-mono shadow-[0_20px_40px_-15px_rgba(0,0,0,0.7)] hover:border-white/20"
          value={address}
          onChange={(e) => setAddress(e.target.value)}
        />
        
        <div className="absolute right-3 top-1/2 -translate-y-1/2">
          <Button 
            type="submit" 
            disabled={loading || !address.trim()} 
            className="h-14 rounded-xl bg-white text-black hover:bg-slate-200 px-8 font-sans font-bold text-lg transition-all duration-300 shadow-[0_0_20px_rgba(255,255,255,0.15)] hover:shadow-[0_0_30px_rgba(255,255,255,0.3)] hover:scale-[1.02] disabled:opacity-30 disabled:hover:scale-100"
          >
            {loading ? <Loader2 className="w-6 h-6 animate-spin text-black" /> : "Run Analysis"}
          </Button>
        </div>
      </form>

      {/* Results Section */}
      {error && (
        <div className="p-5 rounded-2xl bg-red-950/40 border border-red-500/20 text-center text-red-400 font-sans tracking-wide shadow-lg animate-in fade-in slide-in-from-top-4">
          {error}
        </div>
      )}

      {result && (
        <div className="relative animate-in fade-in zoom-in-95 duration-700">
          {/* Glowing backdrop matching result */}
          <div className={`absolute inset-0 blur-3xl opacity-20 pointer-events-none ${
            result.verdict.toUpperCase() === "SAFE" ? "bg-emerald-500" : "bg-rose-500"
          }`} />
          
          <div className={`relative p-10 sm:p-12 rounded-3xl border backdrop-blur-2xl flex flex-col items-center space-y-8 shadow-2xl ${
            result.verdict.toUpperCase() === "SAFE" 
              ? "bg-[#021008]/80 border-emerald-500/20" 
              : "bg-[#100303]/80 border-rose-500/20"
          }`}>
            <div className="relative">
              <div className={`absolute inset-0 blur-xl opacity-50 ${result.verdict.toUpperCase() === "SAFE" ? "bg-emerald-400" : "bg-rose-500"}`} />
              {result.verdict.toUpperCase() === "SAFE" ? (
                 <ShieldCheck className="relative w-24 h-24 text-emerald-400" />
              ) : (
                 <ShieldAlert className="relative w-24 h-24 text-rose-500 animate-pulse" />
              )}
            </div>
            
            <div className="text-center space-y-4 w-full">
              <h3 className={`text-5xl sm:text-6xl font-heading font-black tracking-widest uppercase ${
                result.verdict.toUpperCase() === "SAFE" ? "text-emerald-400" : "text-rose-500"
              }`}>{result.verdict}</h3>
              
              <div className="h-px w-32 bg-white/10 mx-auto my-6" />
              
              <div className="flex flex-col items-center">
                <p className="text-slate-400 font-sans text-sm tracking-widest uppercase mb-2">Overall Intelligence Score</p>
                <div className="flex items-baseline space-x-1">
                  <span className="font-mono text-5xl sm:text-6xl font-bold text-white tracking-tighter">{result.score}</span>
                  <span className="font-mono text-2xl text-slate-500">/100</span>
                </div>
              </div>

              {result.reason && (
                <div className="w-full p-6 rounded-2xl bg-white/5 border border-white/5 text-left space-y-3">
                  <p className="text-[10px] font-mono text-slate-500 uppercase tracking-widest">Narrative Reason</p>
                  <p className="text-sm text-slate-300 font-sans leading-relaxed">{result.reason}</p>
                </div>
              )}

              <div className="grid grid-cols-2 gap-4 w-full">
                <div className="p-4 rounded-xl bg-white/5 border border-white/5 text-left">
                  <p className="text-[10px] font-mono text-slate-500 uppercase mb-1">Creator Reputation</p>
                  <p className={`font-bold text-sm ${result.creator_reputation === "TRUSTED" ? "text-emerald-400" : "text-rose-500"}`}>
                    {result.creator_reputation || (result.teaser ? "RESTRICTED" : "UNKNOWN")}
                  </p>
                </div>
                <div className="p-4 rounded-xl bg-white/5 border border-white/5 text-left">
                  <p className="text-[10px] font-mono text-slate-500 uppercase mb-1">Insider Risk</p>
                  <p className={`font-bold text-sm ${result.insider_risk === "Low" ? "text-emerald-400" : "text-rose-500"}`}>
                    {result.insider_risk || (result.teaser ? "RESTRICTED" : "NORMAL")}
                  </p>
                </div>
              </div>
            </div>

            {/* Intelligent Blur Section (PR-NEXUS-AUTH-JOURNEY) */}
            {result.teaser && (
              <div className="w-full mt-8 space-y-6">
                <div className="relative p-6 rounded-2xl bg-white/5 border border-white/10 overflow-hidden group/blur">
                  <div className="absolute inset-0 bg-linear-to-b from-transparent to-[#030508] z-10" />
                  <div className="space-y-4 filter blur-sm select-none opacity-40">
                    <div className="flex items-center justify-between">
                      <div className="h-4 w-32 bg-slate-700 rounded animate-pulse" />
                      <div className="h-4 w-12 bg-slate-700 rounded animate-pulse" />
                    </div>
                    <div className="h-20 w-full bg-slate-800/50 rounded-xl" />
                    <div className="flex space-x-2">
                       <div className="h-8 w-20 bg-slate-700 rounded-lg" />
                       <div className="h-8 w-24 bg-slate-700 rounded-lg" />
                    </div>
                  </div>
                  
                  <div className="absolute inset-0 z-20 flex flex-col items-center justify-center p-6 text-center space-y-4">
                    <div className="w-12 h-12 rounded-full bg-blue-500/20 flex items-center justify-center text-blue-400 border border-blue-500/30">
                       <ShieldCheck className="w-6 h-6" />
                    </div>
                    <p className="text-sm font-sans text-slate-300 max-w-[240px]">
                      Creator history and insider analysis are <strong>restricted</strong>.
                    </p>
                    <Button 
                      onClick={() => window.location.href = "/sign-up"}
                      className="h-10 rounded-lg bg-blue-600 hover:bg-blue-500 text-white text-xs font-bold px-6 shadow-lg shadow-blue-500/20 transition-all hover:scale-105"
                    >
                      Login to Unlock Full Intelligence
                    </Button>
                  </div>
                </div>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
