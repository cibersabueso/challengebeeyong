import { useState } from "react";
import type { Item } from "../api/items";

interface ItemCardProps {
  item: Item;
  onReserve: (itemId: string, quantity: number) => void;
  isReserving: boolean;
}

export function ItemCard({ item, onReserve, isReserving }: ItemCardProps) {
  const [quantity, setQuantity] = useState<number>(1);
  const outOfStock = item.available <= 0;
  const ratio = item.total > 0 ? item.available / item.total : 0;
  const percent = Math.round(ratio * 100);

  const initial = item.name.charAt(0).toUpperCase();

  function clampedSet(next: number) {
    if (next < 1) {
      setQuantity(1);
      return;
    }
    if (next > item.available) {
      setQuantity(Math.max(1, item.available));
      return;
    }
    setQuantity(next);
  }

  function handleReserve() {
    if (outOfStock || isReserving) return;
    onReserve(item.id, quantity);
  }

  return (
    <div className="flex h-full flex-col rounded-xl border border-slate-200 bg-white p-4 shadow-sm transition hover:shadow-md">
      <div className="flex items-start gap-3">
        <div className="flex h-12 w-12 flex-none items-center justify-center rounded-full bg-slate-900 text-lg font-semibold text-white">
          {initial}
        </div>
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-semibold text-slate-900">{item.name}</p>
          <p className="mt-0.5 text-xs text-slate-500">Total Stock Meter</p>
        </div>
      </div>

      <div className="mt-3">
        <div className="h-1.5 w-full overflow-hidden rounded-full bg-slate-100">
          <div
            className={`h-full rounded-full ${
              outOfStock ? "bg-slate-300" : "bg-brand-600"
            }`}
            style={{ width: `${Math.max(0, Math.min(100, percent))}%` }}
          />
        </div>
        <div className="mt-2 flex items-center justify-between text-xs">
          <span className="text-slate-600">
            {item.available} / {item.total} {outOfStock ? "Out of Stock" : "Available"}
          </span>
          <span className="text-slate-400">{percent}%</span>
        </div>
      </div>

      <div className="mt-4 flex items-center gap-2">
        <button
          type="button"
          onClick={() => clampedSet(quantity - 1)}
          disabled={outOfStock || quantity <= 1 || isReserving}
          className="h-8 w-8 rounded-md border border-slate-200 text-slate-700 disabled:cursor-not-allowed disabled:opacity-40"
          aria-label="Decrease quantity"
        >
          −
        </button>
        <input
          type="number"
          min={1}
          max={Math.max(1, item.available)}
          value={quantity}
          onChange={(e) => clampedSet(Number(e.target.value))}
          disabled={outOfStock || isReserving}
          className="h-8 w-14 rounded-md border border-slate-200 text-center text-sm text-slate-900 disabled:bg-slate-50 disabled:text-slate-400"
        />
        <button
          type="button"
          onClick={() => clampedSet(quantity + 1)}
          disabled={outOfStock || quantity >= item.available || isReserving}
          className="h-8 w-8 rounded-md border border-slate-200 text-slate-700 disabled:cursor-not-allowed disabled:opacity-40"
          aria-label="Increase quantity"
        >
          +
        </button>
      </div>

      <button
        type="button"
        onClick={handleReserve}
        disabled={outOfStock || isReserving}
        className={`mt-3 w-full rounded-md py-2 text-sm font-medium transition ${
          outOfStock
            ? "cursor-not-allowed bg-slate-100 text-slate-400"
            : isReserving
            ? "cursor-wait bg-brand-500 text-white"
            : "bg-slate-900 text-white hover:bg-slate-800"
        }`}
      >
        {outOfStock ? "Out of Stock" : isReserving ? "Reserving..." : "Reserve Item"}
      </button>
    </div>
  );
}
