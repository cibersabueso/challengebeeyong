import { useQuery } from "@tanstack/react-query";
import { fetchItems, type Item } from "../api/items";
import { useUserId } from "./useUserId";

const POLL_INTERVAL_MS = 2000;

export function useItems() {
  const userId = useUserId();
  return useQuery<Item[], Error>({
    queryKey: ["items"],
    queryFn: () => fetchItems(userId),
    refetchInterval: POLL_INTERVAL_MS,
  });
}
