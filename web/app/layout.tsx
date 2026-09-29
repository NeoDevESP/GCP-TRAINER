import type { Metadata, Viewport } from "next";
import "./globals.css";
import "@xterm/xterm/css/xterm.css";

export const metadata: Metadata = {
  title: "Cloud Mastery — Simulador de laboratorios de Google Cloud",
  description:
    "Aprende Google Cloud trabajando: construye, rompe, diagnostica y domina en una nube simulada, una empresa persistente y entornos reales.",
};

export const viewport: Viewport = { width: "device-width", initialScale: 1 };

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="es">
      <head>
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="" />
        {/* Roboto for the graphical console; without internet (desktop app) it falls back to system fonts. */}
        <link href="https://fonts.googleapis.com/css2?family=Roboto:wght@400;500;700&family=Roboto+Mono&display=swap" rel="stylesheet" />
      </head>
      <body>{children}</body>
    </html>
  );
}
