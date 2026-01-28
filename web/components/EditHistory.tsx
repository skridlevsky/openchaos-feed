"use client";

import { useState } from "react";
import type { EditHistoryEntry } from "@/lib/types";

function timeAgo(dateStr: string): string {
  const seconds = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000);
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  return `${Math.floor(days / 30)}mo ago`;
}

export function EditHistory({ entries }: { entries: EditHistoryEntry[] }) {
  const [open, setOpen] = useState(false);

  if (entries.length === 0) return null;

  return (
    <div className="mt-1">
      <button
        onClick={() => setOpen(!open)}
        className="text-xs text-zinc-600 hover:text-zinc-400 transition-colors flex items-center gap-1"
      >
        <span className="text-[10px]">{open ? "\u25BE" : "\u25B8"}</span>
        edited {entries.length === 1 ? "once" : `${entries.length} times`}
      </button>
      {open && (
        <div className="mt-1.5 ml-2 border-l-2 border-zinc-800 pl-3 space-y-2">
          {entries.map((entry, i) => (
            <div key={i}>
              <div className="text-[10px] text-zinc-600 font-mono mb-0.5">
                {i === entries.length - 1 ? "original" : `edit ${entries.length - i}`}
                {" \u00B7 "}
                {timeAgo(entry.editedAt)}
              </div>
              <div className="text-xs text-zinc-500 whitespace-pre-line break-words line-clamp-4">
                {entry.body}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
