import type { Metadata } from "next";
import { Analytics } from "@vercel/analytics/react";
import "./globals.css";

export const metadata: Metadata = {
  title: {
    default: "OpenChaos Feed — Governance Activity",
    template: "%s | OpenChaos Feed",
  },
  description:
    "Public record of governance activity for the OpenChaos open source project. Every PR vote, comment, and contribution — searchable and transparent.",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en" className="dark">
      <body className="bg-zinc-950 text-zinc-100 min-h-screen flex flex-col antialiased">
        <nav className="border-b border-zinc-800 bg-zinc-950/80 backdrop-blur-sm sticky top-0 z-50">
          <div className="max-w-6xl mx-auto px-4 h-14 flex items-center justify-between">
            <a href="/" className="font-bold text-lg tracking-tight">
              <span>Open</span>
              <span>Chaos</span>
              <span className="text-zinc-500 ml-1.5 font-normal text-sm">feed</span>
            </a>
            <div className="flex items-center gap-6 text-sm">
              <a href="/" className="text-zinc-400 hover:text-zinc-100 transition-colors">
                Activity
              </a>
              <a href="/voters" className="text-zinc-400 hover:text-zinc-100 transition-colors">
                Voters
              </a>
              <a
                href="https://github.com/skridlevsky/openchaos"
                target="_blank"
                rel="noopener noreferrer"
                className="text-zinc-400 hover:text-zinc-100 transition-colors"
              >
                GitHub
              </a>
            </div>
          </div>
        </nav>
        <main className="flex-1 max-w-6xl mx-auto w-full px-4 py-8">
          {children}
        </main>
        <footer className="border-t border-zinc-800 py-8 mt-16">
          <div className="max-w-6xl mx-auto px-4 flex flex-col sm:flex-row items-center justify-between gap-4 text-sm text-zinc-500">
            <div>
              Open governance data for{" "}
              <a
                href="https://github.com/skridlevsky/openchaos"
                className="text-zinc-400 hover:text-zinc-100 transition-colors"
              >
                skridlevsky/openchaos
              </a>
            </div>
            <div className="flex items-center gap-4">
              <a
                href="https://github.com/skridlevsky/openchaos-feed"
                className="hover:text-zinc-100 transition-colors"
              >
                Source
              </a>
            </div>
          </div>
          <div className="max-w-6xl mx-auto px-4 mt-4 text-center text-xs text-zinc-600">
            Analytics by{" "}
            <a
              href="https://vercel.com/analytics"
              className="text-zinc-500 hover:text-zinc-400 transition-colors"
            >
              Vercel
            </a>
            {" · No cookies · No PII"}
          </div>
        </footer>
        <Analytics />
      </body>
    </html>
  );
}
