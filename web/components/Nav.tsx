"use client";

import { useEffect, useState } from "react";
import { api, setToken } from "@/lib/api";
import type { User } from "@/lib/types";

const links = [
  ["/dashboard", "Dashboard"],
  ["/catalog", "Labs"],
  ["/incidents", "Incidents"],
  ["/company", "Company"],
  ["/skills", "Skill graph"],
  ["/leaderboard", "Leagues"],
];

export default function Nav({ active }: { active?: string }) {
  const [user, setUser] = useState<User | null>(null);
  useEffect(() => {
    api<{ user: User }>("/api/me")
      .then((p) => setUser(p.user))
      .catch(() => setUser(null));
  }, []);
  return (
    <nav className="nav" aria-label="Main">
      <a className="skip-link" href="#main">Skip to content</a>
      <a className="brand" href="/dashboard">
        Cloud <span>Mastery</span>
      </a>
      {links.map(([href, label]) => (
        <a key={href} href={href} className={active === href ? "active" : ""} aria-current={active === href ? "page" : undefined}>
          {label}
        </a>
      ))}
      {user && (user.role === "admin" || user.role === "instructor") && (
        <a href="/admin" className={active === "/admin" ? "active" : ""} aria-current={active === "/admin" ? "page" : undefined}>
          Instructor
        </a>
      )}
      <div className="spacer" />
      {user && <span className="muted small">{user.name}</span>}
      <button
        className="btn secondary"
        onClick={() => {
          setToken("");
          window.location.href = "/";
        }}
      >
        Sign out
      </button>
    </nav>
  );
}
