import type { Item } from "../api/items";
import { ItemCard } from "./ItemCard";

interface InventoryGridProps {
  items: Item[];
  onReserve: (itemId: string, quantity: number) => void;
  reservingItemId: string | null;
}

export function InventoryGrid({ items, onReserve, reservingItemId }: InventoryGridProps) {
  if (items.length === 0) {
    return (
      <div className="rounded-xl border border-dashed border-slate-200 bg-white py-12 text-center text-sm text-slate-500">
        No items available.
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {items.map((it) => (
        <ItemCard
          key={it.id}
          item={it}
          onReserve={onReserve}
          isReserving={reservingItemId === it.id}
        />
      ))}
    </div>
  );
}
