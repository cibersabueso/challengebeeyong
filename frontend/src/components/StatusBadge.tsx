interface StatusBadgeProps {
  isLive: boolean;
  lastUpdatedAt?: number;
}

export function StatusBadge({ isLive, lastUpdatedAt }: StatusBadgeProps) {
  const tooltip =
    lastUpdatedAt && lastUpdatedAt > 0
      ? `Last updated: ${new Date(lastUpdatedAt).toLocaleTimeString()}`
      : "Polling every 2 seconds";

  return (
    <span
      title={tooltip}
      className="inline-flex items-center gap-1.5 rounded-full border border-slate-200 bg-white px-3 py-1 text-xs font-medium"
    >
      <span
        className={`h-2 w-2 rounded-full ${
          isLive ? "bg-emerald-500" : "bg-slate-300"
        }`}
        aria-hidden
      />
      <span className={isLive ? "text-slate-700" : "text-slate-400"}>
        {isLive ? "Live" : "Offline"}
      </span>
    </span>
  );
}
