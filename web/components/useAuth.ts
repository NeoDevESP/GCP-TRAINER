"use client";

import { useEffect } from "react";
import { isSignedIn } from "@/lib/api";

// useAuth sends visitors without a session back to the sign-in page.
export function useAuth() {
  useEffect(() => {
    if (!isSignedIn()) window.location.href = "/";
  }, []);
}
