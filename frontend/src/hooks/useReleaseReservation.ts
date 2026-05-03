import { useMutation, useQueryClient } from "@tanstack/react-query";
import { releaseReservation, type ReleaseResponse } from "../api/reservations";
import { useUserId } from "./useUserId";

export function useReleaseReservation() {
  const userId = useUserId();
  const qc = useQueryClient();

  return useMutation<ReleaseResponse, Error, string>({
    mutationFn: (reservationId) => releaseReservation(userId, reservationId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["items"] });
      void qc.invalidateQueries({ queryKey: ["reservations", "mine", userId] });
    },
  });
}
