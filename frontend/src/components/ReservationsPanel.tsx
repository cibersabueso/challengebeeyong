import type { Reservation } from "../api/reservations";
import type { Item } from "../api/items";
import { ReservationItem } from "./ReservationItem";

interface ReservationsPanelProps {
  reservations: Reservation[];
  items: Item[];
  onRelease: (reservationId: string) => void;
  releasingId: string | null;
  isLoading: boolean;
}

export function ReservationsPanel({
  reservations,
  items,
  onRelease,
  releasingId,
  isLoading,
}: ReservationsPanelProps) {
  const itemById = new Map(items.map((it) => [it.id, it]));

  return (
    <aside className="flex h-full flex-col rounded-xl border border-slate-200 bg-slate-50 p-4">
      <h2 className="text-sm font-semibold text-slate-900">Your Reservations</h2>
      <p className="mt-0.5 text-xs text-slate-500">Active holds, expire after 60s.</p>

      <div className="mt-4 flex-1 space-y-2 overflow-y-auto">
        {isLoading && reservations.length === 0 ? (
          <p className="text-xs text-slate-400">Loading...</p>
        ) : reservations.length === 0 ? (
          <div className="rounded-lg border border-dashed border-slate-200 bg-white py-6 text-center text-xs text-slate-400">
            No active reservations.
          </div>
        ) : (
          reservations.map((res) => (
            <ReservationItem
              key={res.id}
              reservation={res}
              item={itemById.get(res.item_id)}
              onRelease={onRelease}
              isReleasing={releasingId === res.id}
            />
          ))
        )}
      </div>
    </aside>
  );
}
