import { useEffect } from "react";

interface ToastProps {
  title: string;
  description?: string;
  onDismiss: () => void;
  durationMs?: number;
}

export function Toast({ title, description, onDismiss, durationMs = 4000 }: ToastProps) {
  useEffect(() => {
    const id = window.setTimeout(onDismiss, durationMs);
    return () => window.clearTimeout(id);
  }, [onDismiss, durationMs]);

  return (
    <div className="pointer-events-auto w-80 rounded-lg border border-red-200 bg-red-50 px-4 py-3 shadow-lg">
      <div className="flex items-start gap-3">
        <div className="flex h-6 w-6 flex-none items-center justify-center rounded-full bg-red-500 text-white">
          <span className="text-xs font-bold">!</span>
        </div>
        <div className="flex-1">
          <p className="text-sm font-semibold text-red-900">{title}</p>
          {description ? (
            <p className="mt-0.5 text-xs text-red-800">{description}</p>
          ) : null}
        </div>
        <button
          type="button"
          onClick={onDismiss}
          className="text-red-500 transition hover:text-red-700"
          aria-label="Dismiss"
        >
          ×
        </button>
      </div>
    </div>
  );
}

interface ToastStackProps {
  children: React.ReactNode;
}

export function ToastStack({ children }: ToastStackProps) {
  return (
    <div className="pointer-events-none fixed right-6 top-6 z-50 flex flex-col gap-2">
      {children}
    </div>
  );
}
