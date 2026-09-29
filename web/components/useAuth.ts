"use client";

import { useEffect } from "react";
import { getToken } from "@/lib/api";

// useAuth sends visitors without a token back to the sign-in page.
export function useAuth() {
  useEffect(() => {
    if (!getToken()) window.location.href = "/";
  }, []);
}
