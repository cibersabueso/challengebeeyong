import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createReservation, type CreateReservationInput, type Reservation } from "../api/reservations";
import { newUuidV4 } from "../lib/uuid";
import { useUserId } from "./useUserId";

export function useCreateReservation() {
  const userId = useUserId();
  const qc = useQueryClient();

  return useMutation<Reservation, Error, CreateReservationInput>({
    mutationFn: (input) => {
      const idempotencyKey = newUuidV4();
      return createReservation(userId, idempotencyKey, input);
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["items"] });
      void qc.invalidateQueries({ queryKey: ["reservations", "mine", userId] });
    },
  });
}
