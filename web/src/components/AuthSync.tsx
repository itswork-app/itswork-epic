"use client";

import { useEffect, useRef } from "react";
import { useAuth, useUser } from "@clerk/nextjs";

export function AuthSync() {
  const { isSignedIn, getToken } = useAuth();
  const { user } = useUser();
  const syncedRef = useRef<string | null>(null);

  useEffect(() => {
    async function sync() {
      if (isSignedIn && user && syncedRef.current !== user.id) {
        try {
          const token = await getToken();
          const res = await fetch("/api/v1/auth/sync", {
            method: "POST",
            headers: {
              Authorization: `Bearer ${token}`,
            },
          });
          
          if (res.ok) {
            const data = await res.json();
            console.log("User Synced Successfully:", data.role);
            syncedRef.current = user.id;

            // Redirect Logic (PR-POST-LOGIN-REDIRECT)
            // Only redirect if we are on the landing page
            if (window.location.pathname === "/") {
              const isProd = window.location.hostname !== 'localhost';
              if (data.role === "unassigned" || !data.role) {
                window.location.href = "/onboarding";
              } else {
                const targetUrl = data.role === "trader" 
                  ? (isProd ? 'https://trader.itswork.app' : 'http://localhost:3000') 
                  : (isProd ? 'https://dev.itswork.app' : 'http://localhost:3000');
                window.location.href = targetUrl;
              }
            }
          }
        } catch (err) {
          console.error("Auth sync failed:", err);
        }
      }
    }

    sync();
  }, [isSignedIn, user, getToken]);

  return null;
}
