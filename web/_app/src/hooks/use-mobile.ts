import * as React from "react";

const MOBILE_BREAKPOINT = 768;
const QUERY = `(max-width: ${String(MOBILE_BREAKPOINT - 1)}px)`;

const subscribe = (onChange: () => void) => {
  const mql = window.matchMedia(QUERY);
  mql.addEventListener("change", onChange);
  return () => {
    mql.removeEventListener("change", onChange);
  };
};

/** Whether the viewport is a phone's: the browser's own fact, false before hydration. */
export function useIsMobile(): boolean {
  return React.useSyncExternalStore(
    subscribe,
    () => window.matchMedia(QUERY).matches,
    () => false,
  );
}
