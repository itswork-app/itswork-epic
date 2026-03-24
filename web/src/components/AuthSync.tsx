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
            console.log("User Synced Successfully");
            syncedRef.current = user.id;
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
