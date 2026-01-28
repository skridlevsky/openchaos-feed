"use client";

import { useState, useRef, useEffect } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeRaw from "rehype-raw";

const COLLAPSED_HEIGHT = 96; // ~6 lines at text-sm

// Only allow GitHub-hosted content to prevent phishing / abuse.
// External links are redacted; users can view them on GitHub.
const ALLOWED_HOSTS = ["githubusercontent.com", "github.com"];

function isAllowedHost(url: string): boolean {
  try {
    const { hostname } = new URL(url);
    return ALLOWED_HOSTS.some(
      (h) => hostname === h || hostname.endsWith(`.${h}`)
    );
  } catch {
    return false;
  }
}

export function ExpandableText({ text }: { text: string }) {
  const [expanded, setExpanded] = useState(false);
  const [needsTruncation, setNeedsTruncation] = useState(false);
  const [lightboxSrc, setLightboxSrc] = useState<string | null>(null);
  const contentRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (contentRef.current) {
      setNeedsTruncation(contentRef.current.scrollHeight > COLLAPSED_HEIGHT);
    }
  }, [text]);

  // Close lightbox on Escape key
  useEffect(() => {
    if (!lightboxSrc) return;
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setLightboxSrc(null);
    };
    window.addEventListener("keydown", handleKey);
    return () => window.removeEventListener("keydown", handleKey);
  }, [lightboxSrc]);

  return (
    <div className="mt-1.5">
      <div
        ref={contentRef}
        className="relative overflow-hidden"
        style={
          !expanded && needsTruncation
            ? { maxHeight: COLLAPSED_HEIGHT }
            : undefined
        }
      >
        <div className="prose-feed">
          <ReactMarkdown
            remarkPlugins={[remarkGfm]}
            rehypePlugins={[rehypeRaw]}
            components={{
              img: ({ src, alt }) => {
                if (!src || typeof src !== "string" || !isAllowedHost(src)) return null;
                return (
                  <img
                    src={src}
                    alt={typeof alt === "string" ? alt : ""}
                    className="cursor-pointer hover:opacity-80 transition-opacity"
                    onClick={() => setLightboxSrc(src)}
                  />
                );
              },
              a: ({ href, children }) => {
                if (!href || (typeof href === "string" && !isAllowedHost(href))) {
                  return (
                    <span className="text-zinc-600 italic" title="External link redacted — view on GitHub">
                      [{children}]
                    </span>
                  );
                }
                return (
                  <a href={href} target="_blank" rel="noopener noreferrer">
                    {children}
                  </a>
                );
              },
            }}
          >
            {text}
          </ReactMarkdown>
        </div>
        {!expanded && needsTruncation && (
          <div className="absolute bottom-0 left-0 right-0 h-10 bg-gradient-to-t from-zinc-950 to-transparent" />
        )}
      </div>
      {needsTruncation && (
        <button
          onClick={() => setExpanded(!expanded)}
          className="text-xs text-zinc-500 hover:text-zinc-300 mt-1 transition-colors"
        >
          {expanded ? "show less" : "read more"}
        </button>
      )}
      {lightboxSrc && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm cursor-pointer"
          onClick={() => setLightboxSrc(null)}
        >
          <img
            src={lightboxSrc}
            alt=""
            className="max-w-[90vw] max-h-[90vh] object-contain rounded-lg"
            onClick={(e) => e.stopPropagation()}
          />
        </div>
      )}
    </div>
  );
}
