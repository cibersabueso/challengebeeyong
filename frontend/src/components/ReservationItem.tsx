import { useCountdown, formatCountdown } from "../hooks/useCountdown";
import type { Reservation } from "../api/reservations";
import type { Item } from "../api/items";

interface ReservationItemProps {
  reservation: Reservation;
  item?: Item;
  onRelease: (reservationId: string) => void;
  isReleasing: boolean;
}

function shortId(id: string): string {
  return id.slice(0, 4).toUpperCase();
}

export function ReservationItem({ reservation, item, onRelease, isReleasing }: ReservationItemProps) {
  const seconds = useCountdown(reservation.expires_at);
  const expired = seconds <= 0;
  const label = item?.name ?? reservation.item_id.slice(0, 8);

  return (
    <div className="rounded-lg border border-slate-200 bg-white p-3 shadow-sm">
      <div className="flex items-start justify-between">
        <div className="min-w-0">
          <p className="truncate text-sm font-semibold text-slate-900">{label}</p>
          <p className="mt-0.5 text-[10px] uppercase tracking-wide text-slate-400">
            #RES-{shortId(reservation.id)}
          </p>
        </div>
        <span className="text-[10px] uppercase tracking-wide text-slate-400">Countdown</span>
      </div>

      <p
        className={`mt-2 font-mono text-2xl font-bold ${
          expired ? "text-slate-300" : seconds <= 10 ? "text-red-500" : "text-amber-500"
        }`}
      >
        {formatCountdown(seconds)}
      </p>

      <div className="mt-2 flex items-center justify-between">
        <p className="text-xs text-slate-500">
          {reservation.quantity} Unit{reservation.quantity === 1 ? "" : "s"} Held
        </p>
        <button
          type="button"
          onClick={() => onRelease(reservation.id)}
          disabled={isReleasing}
          className="rounded-md border border-slate-200 px-2 py-1 text-xs text-slate-700 transition hover:bg-slate-50 disabled:cursor-wait disabled:opacity-60"
        >
          {isReleasing ? "Releasing..." : "Release"}
        </button>
      </div>
    </div>
  );
}
