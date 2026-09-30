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
        {/* Fonts ship with the app (public/fonts): Roboto for the Google Cloud console replica,
            Bricolage Grotesque and IBM Plex for the learning pages. They work offline. */}
        <link rel="stylesheet" href="/fonts/fonts.css" />
      </head>
      <body>{children}</body>
    </html>
  );
}
