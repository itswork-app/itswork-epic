"use client";

import { useEffect, useState } from "react";
import { useAuth } from "@clerk/nextjs";
import { Loader2, Zap } from "lucide-react";

interface QuotaData {
  free_ui: number;
  free_ui_max: number;
  free_api: number;
  free_api_max: number;
  subscription: number;
}

export function QuotaLimit() {
  const { getToken, isSignedIn } = useAuth();
  const [quota, setQuota] = useState<QuotaData | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function fetchQuota() {
      if (!isSignedIn) return;
      try {
        const token = await getToken();
        const baseUrl = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';
        const res = await fetch(`${baseUrl}/api/v1/user/quota`, {
          headers: { Authorization: `Bearer ${token}` },
        });
        if (res.ok) {
          const data = await res.json();
          setQuota(data);
        }
      } catch (err) {
        console.error("Failed to fetch quota:", err);
      } finally {
        setLoading(false);
      }
    }

    fetchQuota();
  }, [isSignedIn, getToken]);

  if (!isSignedIn) return null;

  if (loading) {
    return (
      <div className="p-4 rounded-2xl bg-white/5 border border-white/10 flex items-center justify-center">
        <Loader2 className="w-4 h-4 animate-spin text-slate-500" />
      </div>
    );
  }

  if (!quota) return null;

  const used = quota.free_ui;
  const max = quota.free_ui_max;
  const percentage = Math.min((used / max) * 100, 100);

  return (
    <div className="p-4 rounded-2xl bg-white/5 border border-white/10 space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center space-x-2">
          <Zap className="w-4 h-4 text-blue-400" />
          <span className="text-xs font-mono font-bold tracking-widest uppercase text-slate-400">Scan Quota</span>
        </div>
        <span className="text-[10px] font-mono text-slate-500">{used}/{max} Used</span>
      </div>
      
      <div className="h-1.5 w-full bg-white/5 rounded-full overflow-hidden">
        <div 
          className="h-full bg-linear-to-r from-blue-600 to-indigo-500 transition-all duration-1000"
          style={{ width: `${percentage}%` }}
        />
      </div>

      {quota.subscription > 0 && (
        <p className="text-[10px] font-mono text-blue-400/80 text-center animate-pulse">
          + {quota.subscription} Professional Tokens Remaining
        </p>
      )}
    </div>
  );
}
