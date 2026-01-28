"use client";

import { useState } from "react";

export function CollapsibleSection({
  title,
  defaultOpen = false,
  count,
  children,
}: {
  title: string;
  defaultOpen?: boolean;
  count?: number;
  children: React.ReactNode;
}) {
  const [open, setOpen] = useState(defaultOpen);

  return (
    <div>
      <button
        onClick={() => setOpen(!open)}
        className="flex items-center gap-2 text-xl font-bold mb-4 hover:text-zinc-300 transition-colors group"
      >
        <span
          className="text-sm text-zinc-500 group-hover:text-zinc-400 transition-transform inline-block"
          style={{ transform: open ? "rotate(90deg)" : "rotate(0deg)" }}
        >
          &#9656;
        </span>
        {title}
        {count !== undefined && (
          <span className="text-sm font-normal text-zinc-500">({count})</span>
        )}
      </button>
      {open && children}
    </div>
  );
}
