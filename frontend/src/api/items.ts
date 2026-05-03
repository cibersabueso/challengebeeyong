import { apiRequest } from "./client";

export interface Item {
  id: string;
  name: string;
  total: number;
  reserved: number;
  available: number;
  created_at?: string;
}

export function fetchItems(userId: string): Promise<Item[]> {
  return apiRequest<Item[]>("/items", { method: "GET", userId });
}
