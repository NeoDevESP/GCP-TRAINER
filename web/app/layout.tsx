import type { Metadata, Viewport } from "next";
import "./globals.css";
import "@xterm/xterm/css/xterm.css";

export const metadata: Metadata = {
  title: "Cloud Mastery — GCP Lab Simulator",
  description: "Learn Google Cloud by working: build, break, diagnose and master in a simulated cloud, a persistent company and real sandboxes.",
};

export const viewport: Viewport = { width: "device-width", initialScale: 1 };

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
