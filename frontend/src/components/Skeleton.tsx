export function ItemCardSkeleton() {
  return (
    <div className="rounded-xl border border-slate-200 bg-white p-4">
      <div className="flex items-start gap-3">
        <div className="h-12 w-12 animate-pulse rounded-full bg-slate-200" />
        <div className="flex-1 space-y-2">
          <div className="h-3 w-32 animate-pulse rounded bg-slate-200" />
          <div className="h-2.5 w-20 animate-pulse rounded bg-slate-100" />
        </div>
      </div>
      <div className="mt-3 h-1.5 w-full animate-pulse rounded-full bg-slate-100" />
      <div className="mt-2 h-3 w-24 animate-pulse rounded bg-slate-100" />
      <div className="mt-4 h-8 w-full animate-pulse rounded bg-slate-100" />
      <div className="mt-3 h-9 w-full animate-pulse rounded bg-slate-200" />
    </div>
  );
}

export function InventoryGridSkeleton() {
  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {[1, 2, 3, 4, 5, 6].map((i) => (
        <ItemCardSkeleton key={i} />
      ))}
    </div>
  );
}
