"use client";

import { useState } from "react";
import { Search, ShieldAlert, ShieldCheck, Loader2 } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";

export function SearchBar() {
  const [address, setAddress] = useState("");
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<{ score: number; verdict: string } | null>(null);
  const [error, setError] = useState("");

  const handleSearch = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!address.trim()) return;

    setLoading(true);
    setResult(null);
    setError("");

    try {
      // Due to Next.js API Proxy we hit the route seamlessly
      const res = await fetch(`/api/v1/token/${address.trim()}`);
      if (!res.ok) {
        throw new Error("Failed to fetch token intelligence");
      }
      const data = await res.json();
      setResult({
        score: data.score || 0,
        verdict: data.verdict || "UNKNOWN",
      });
    } catch (err: any) {
      setError(err.message || "Auditing service currently unavailable.");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="w-full max-w-2xl mx-auto space-y-8 animate-in fade-in slide-in-from-bottom-8 duration-700 delay-150">
      <form onSubmit={handleSearch} className="relative flex items-center w-full group">
        <div className="absolute inset-y-0 left-0 flex items-center pl-5 pointer-events-none transition-colors duration-300 group-focus-within:text-blue-400">
          <Search className="w-6 h-6 text-gray-400 transition-colors duration-300 group-focus-within:text-blue-400" />
        </div>
        <Input
          type="text"
          placeholder="Enter Solana mint address (e.g. 7vfCXTUX...)"
          className="w-full h-16 pl-14 pr-36 text-lg bg-[#0a0a1a] border-white/10 text-white placeholder-gray-500 rounded-full focus:ring-4 focus:ring-blue-500/20 focus:border-blue-500/50 transition-all duration-300 font-mono shadow-[0_0_30px_rgba(0,0,0,0.5)]"
          value={address}
          onChange={(e) => setAddress(e.target.value)}
        />
        <Button 
          type="submit" 
          disabled={loading || !address.trim()} 
          className="absolute right-2 h-12 rounded-full bg-blue-600 hover:bg-blue-500 text-white px-8 font-bold text-base transition-all duration-300 shadow-[0_0_15px_rgba(37,99,235,0.4)] hover:shadow-[0_0_20px_rgba(59,130,246,0.6)] disabled:opacity-50 disabled:shadow-none"
        >
          {loading ? <Loader2 className="w-5 h-5 animate-spin" /> : "Analyze"}
        </Button>
      </form>

      {/* Results Section */}
      {error && (
        <div className="p-4 rounded-xl bg-red-500/10 border border-red-500/20 text-center text-red-400 animate-in fade-in slide-in-from-top-4">
          {error}
        </div>
      )}

      {result && (
        <div className={`p-8 rounded-3xl border backdrop-blur-xl flex flex-col items-center space-y-5 animate-in fade-in zoom-in-95 duration-500 ${
          result.verdict.toUpperCase() === "SAFE" 
            ? "bg-green-500/5 border-green-500/20 text-green-400 shadow-[0_0_50px_rgba(34,197,94,0.1)]" 
            : "bg-red-500/5 border-red-500/20 text-red-500 shadow-[0_0_50px_rgba(239,68,68,0.1)]"
        }`}>
          {result.verdict.toUpperCase() === "SAFE" ? (
             <ShieldCheck className="w-20 h-20 drop-shadow-[0_0_25px_rgba(72,187,120,0.6)] animate-in fade-in zoom-in duration-700" />
          ) : (
             <ShieldAlert className="w-20 h-20 drop-shadow-[0_0_25px_rgba(239,68,68,0.6)] animate-pulse" />
          )}
          <div className="text-center space-y-2">
            <h3 className="text-4xl font-black tracking-widest uppercase">{result.verdict}</h3>
            <div className="h-px w-24 bg-current/20 mx-auto my-4" />
            <p className="text-white/70 font-mono text-xl">Intelligence Score: <br/><span className="font-bold text-3xl text-white block mt-2">{result.score}/100</span></p>
          </div>
        </div>
      )}
    </div>
  );
}
