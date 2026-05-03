import { useQuery } from "@tanstack/react-query";
import { listMyReservations, type Reservation } from "../api/reservations";
import { useUserId } from "./useUserId";

const POLL_INTERVAL_MS = 2000;

export function useMyReservations() {
  const userId = useUserId();
  return useQuery<Reservation[], Error>({
    queryKey: ["reservations", "mine", userId],
    queryFn: () => listMyReservations(userId),
    refetchInterval: POLL_INTERVAL_MS,
  });
}
