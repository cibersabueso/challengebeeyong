import { apiRequest } from "./client";

export interface Reservation {
  id: string;
  item_id: string;
  user_id: string;
  quantity: number;
  status: "active" | "released" | "expired";
  expires_at: string;
  created_at: string;
  released_at?: string | null;
}

export interface CreateReservationInput {
  itemId: string;
  quantity: number;
}

export interface ReleaseResponse {
  status: "released" | "already_released";
  reservation_id: string;
}

export function createReservation(
  userId: string,
  idempotencyKey: string,
  input: CreateReservationInput,
): Promise<Reservation> {
  return apiRequest<Reservation>("/reservations", {
    method: "POST",
    userId,
    idempotencyKey,
    body: { item_id: input.itemId, quantity: input.quantity },
  });
}

export function listMyReservations(userId: string): Promise<Reservation[]> {
  return apiRequest<Reservation[]>("/reservations", { method: "GET", userId });
}

export function releaseReservation(userId: string, reservationId: string): Promise<ReleaseResponse> {
  return apiRequest<ReleaseResponse>(`/reservations/${reservationId}`, {
    method: "DELETE",
    userId,
  });
}
