import { useCallback, useState } from "react";
import { useItems } from "./hooks/useItems";
import { useMyReservations } from "./hooks/useMyReservations";
import { useCreateReservation } from "./hooks/useCreateReservation";
import { useReleaseReservation } from "./hooks/useReleaseReservation";
import { ApiError, userMessageFor } from "./lib/errors";
import { InventoryGrid } from "./components/InventoryGrid";
import { ReservationsPanel } from "./components/ReservationsPanel";
import { Toast, ToastStack } from "./components/Toast";
import { StatusBadge } from "./components/StatusBadge";
import { InventoryGridSkeleton } from "./components/Skeleton";

interface ToastEntry {
  id: number;
  title: string;
  description?: string;
}

export default function App() {
  const itemsQuery = useItems();
  const reservationsQuery = useMyReservations();
  const createMutation = useCreateReservation();
  const releaseMutation = useReleaseReservation();

  const [toasts, setToasts] = useState<ToastEntry[]>([]);
  const [reservingItemId, setReservingItemId] = useState<string | null>(null);
  const [releasingId, setReleasingId] = useState<string | null>(null);

  const pushToast = useCallback((title: string, description?: string) => {
    setToasts((curr) => {
      const id = Date.now() + Math.random();
      const next = [...curr, { id, title, description }];
      return next.slice(-3);
    });
  }, []);

  const dismissToast = useCallback((id: number) => {
    setToasts((curr) => curr.filter((t) => t.id !== id));
  }, []);

  const handleReserve = useCallback(
    (itemId: string, quantity: number) => {
      setReservingItemId(itemId);
      createMutation.mutate(
        { itemId, quantity },
        {
          onSuccess: () => {
            setReservingItemId(null);
          },
          onError: (err) => {
            setReservingItemId(null);
            const items = itemsQuery.data ?? [];
            const it = items.find((x) => x.id === itemId);
            if (err instanceof ApiError && err.code === "OUT_OF_STOCK") {
              pushToast(
                "Item Taken",
                it
                  ? `Sorry, the ${it.name} was just reserved by another user.`
                  : "This item was just reserved by another user.",
              );
            } else {
              pushToast("Reservation failed", userMessageFor(err));
            }
          },
        },
      );
    },
    [createMutation, itemsQuery.data, pushToast],
  );

  const handleRelease = useCallback(
    (reservationId: string) => {
      setReleasingId(reservationId);
      releaseMutation.mutate(reservationId, {
        onSuccess: () => {
          setReleasingId(null);
        },
        onError: (err) => {
          setReleasingId(null);
          pushToast("Release failed", userMessageFor(err));
        },
      });
    },
    [releaseMutation, pushToast],
  );

  const items = itemsQuery.data ?? [];
  const reservations = reservationsQuery.data ?? [];
  const itemsLoading = itemsQuery.isLoading;
  const itemsError = itemsQuery.isError;

  return (
    <div className="min-h-screen bg-slate-50 text-slate-900">
      <header className="border-b border-slate-200 bg-white">
        <div className="mx-auto flex max-w-7xl items-center justify-between px-6 py-4">
          <h1 className="text-base font-semibold tracking-tight">Atomic Inventory</h1>
          <div className="flex items-center gap-3">
            <StatusBadge
              isLive={!itemsError}
              lastUpdatedAt={itemsQuery.dataUpdatedAt}
            />
            <button
              type="button"
              onClick={() => {
                void itemsQuery.refetch();
                void reservationsQuery.refetch();
              }}
              className="rounded-md border border-slate-200 px-3 py-1 text-xs text-slate-700 transition hover:bg-slate-50"
            >
              Refresh
            </button>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-7xl px-6 py-8">
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-[1fr_320px]">
          <section>
            <h2 className="mb-4 text-sm font-medium text-slate-700">Available Inventory</h2>
            {itemsError ? (
              <div className="rounded-xl border border-red-200 bg-red-50 p-6 text-sm text-red-800">
                <p className="font-semibold">Failed to load inventory.</p>
                <p className="mt-1 text-xs">
                  {userMessageFor(itemsQuery.error)}
                </p>
                <button
                  type="button"
                  onClick={() => void itemsQuery.refetch()}
                  className="mt-3 rounded-md border border-red-300 bg-white px-3 py-1 text-xs text-red-800 transition hover:bg-red-100"
                >
                  Retry
                </button>
              </div>
            ) : itemsLoading ? (
              <InventoryGridSkeleton />
            ) : (
              <InventoryGrid
                items={items}
                onReserve={handleReserve}
                reservingItemId={reservingItemId}
              />
            )}
          </section>

          <ReservationsPanel
            reservations={reservations}
            items={items}
            onRelease={handleRelease}
            releasingId={releasingId}
            isLoading={reservationsQuery.isLoading}
          />
        </div>
      </main>

      <ToastStack>
        {toasts.map((t) => (
          <Toast
            key={t.id}
            title={t.title}
            description={t.description}
            onDismiss={() => dismissToast(t.id)}
          />
        ))}
      </ToastStack>
    </div>
  );
}
