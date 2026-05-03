import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ItemCard } from "./ItemCard";
import type { Item } from "../api/items";

function buildItem(overrides: Partial<Item> = {}): Item {
  return {
    id: "8f1d2e6a-3c4b-4f1a-9e3a-1a2b3c4d5e6f",
    name: "Vintage Camera",
    total: 20,
    reserved: 6,
    available: 14,
    ...overrides,
  };
}

describe("ItemCard - happy path", () => {
  it("renders item name, available count and reserve button", () => {
    const onReserve = vi.fn();
    const item = buildItem();

    render(<ItemCard item={item} onReserve={onReserve} isReserving={false} />);

    expect(screen.getByText("Vintage Camera")).toBeInTheDocument();
    expect(screen.getByText(/14 \/ 20 Available/i)).toBeInTheDocument();

    const button = screen.getByRole("button", { name: /reserve item/i });
    expect(button).toBeEnabled();
  });

  it("calls onReserve with the selected quantity when Reserve Item is clicked", async () => {
    const user = userEvent.setup();
    const onReserve = vi.fn();
    const item = buildItem({ available: 5, total: 10, reserved: 5 });

    render(<ItemCard item={item} onReserve={onReserve} isReserving={false} />);

    const incrementBtn = screen.getByRole("button", { name: /increase quantity/i });
    await user.click(incrementBtn);
    await user.click(incrementBtn);

    const reserveBtn = screen.getByRole("button", { name: /reserve item/i });
    await user.click(reserveBtn);

    expect(onReserve).toHaveBeenCalledTimes(1);
    expect(onReserve).toHaveBeenCalledWith(item.id, 3);
  });

  it("shows Reserving... label and disables the button while isReserving is true", () => {
    const onReserve = vi.fn();
    const item = buildItem();

    render(<ItemCard item={item} onReserve={onReserve} isReserving={true} />);

    const button = screen.getByRole("button", { name: /reserving\.\.\./i });
    expect(button).toBeDisabled();
  });
});

describe("ItemCard - error and edge states", () => {
  it("renders Out of Stock when available is zero and disables the reserve button", () => {
    const onReserve = vi.fn();
    const item = buildItem({ available: 0, reserved: 12, total: 12 });

    render(<ItemCard item={item} onReserve={onReserve} isReserving={false} />);

    expect(screen.getByText(/0 \/ 12 Out of Stock/i)).toBeInTheDocument();

    const button = screen.getByRole("button", { name: /out of stock/i });
    expect(button).toBeDisabled();

    fireEvent.click(button);
    expect(onReserve).not.toHaveBeenCalled();
  });

  it("clamps quantity to available so the user cannot request more than available stock", async () => {
    const user = userEvent.setup();
    const onReserve = vi.fn();
    const item = buildItem({ available: 2, reserved: 8, total: 10 });

    render(<ItemCard item={item} onReserve={onReserve} isReserving={false} />);

    const incrementBtn = screen.getByRole("button", { name: /increase quantity/i });
    await user.click(incrementBtn);
    await user.click(incrementBtn);
    await user.click(incrementBtn);
    await user.click(incrementBtn);

    const reserveBtn = screen.getByRole("button", { name: /reserve item/i });
    await user.click(reserveBtn);

    expect(onReserve).toHaveBeenCalledTimes(1);
    expect(onReserve).toHaveBeenCalledWith(item.id, 2);
  });

  it("disables the increment button when quantity equals available stock", async () => {
    const user = userEvent.setup();
    const onReserve = vi.fn();
    const item = buildItem({ available: 1, reserved: 9, total: 10 });

    render(<ItemCard item={item} onReserve={onReserve} isReserving={false} />);

    const incrementBtn = screen.getByRole("button", { name: /increase quantity/i });
    expect(incrementBtn).toBeDisabled();

    await user.click(incrementBtn);
    expect(onReserve).not.toHaveBeenCalled();
  });
});
