import { useEffect, useMemo, useRef, useState } from 'react';
import type { ModelDisplayData, TimeWindowData } from './ModelPricingModal';
import {
  UNRESOLVED_TIME_WINDOWS,
  resolveActiveTimeWindowIndex,
  resolveTimeWindowSnapshot,
  timeWindowScheduleSignature,
} from './time-window';

/** useTimeWindowSnapshot tracks active/history membership from one instant and cleans up its timer. */
export function useTimeWindowSnapshot(windows: TimeWindowData[] | undefined) {
  const windowsRef = useRef(windows);
  windowsRef.current = windows;
  const signature = useMemo(() => timeWindowScheduleSignature(windows), [windows]);
  const [snapshot, setSnapshot] = useState(() => ({ signature, ...resolveTimeWindowSnapshot(windows, new Date()) }));

  useEffect(() => {
    if (!signature) return;
    const evaluate = () => {
      const next = resolveTimeWindowSnapshot(windowsRef.current, new Date());
      setSnapshot((previous) =>
        previous.signature === signature &&
        previous.activeIndex === next.activeIndex &&
        previous.historicalIndices.length === next.historicalIndices.length &&
        previous.historicalIndices.every((index, position) => index === next.historicalIndices[position])
          ? previous
          : { signature, ...next }
      );
    };
    evaluate();
    const timer = setInterval(evaluate, 1000);
    return () => clearInterval(timer);
  }, [signature]);

  // A model/schedule replacement must never render indices from the previous list.
  return snapshot.signature === signature ? snapshot : resolveTimeWindowSnapshot(windows, new Date());
}

/** activeDisplayWindow returns the matched window, or the server snapshot when the browser cannot evaluate it. */
export function activeDisplayWindow(
  windows: TimeWindowData[] | undefined,
  index: number,
  fallbackName?: string
): TimeWindowData | undefined {
  return index === UNRESOLVED_TIME_WINDOWS && fallbackName ? windows?.find((window) => window.name === fallbackName) : windows?.[index];
}

/** displayTokenPricing selects the complete active quote without merging price fields in the browser. */
export function displayTokenPricing(model: ModelDisplayData, at: Date | number) {
  const index = typeof at === 'number' ? at : resolveActiveTimeWindowIndex(model.time_windows, at);
  return activeDisplayWindow(model.time_windows, index, model.active_time_window)?.token_pricing ?? model;
}

/** usePricingMinute updates the catalog once per wall-clock minute when it has scheduled quotes. */
export function usePricingMinute(enabled: boolean) {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    if (!enabled) return;
    let timer: ReturnType<typeof setTimeout>;
    const tick = () => {
      const at = new Date();
      setNow(at);
      timer = setTimeout(tick, 60_000 - (at.getTime() % 60_000));
    };
    tick();
    return () => clearTimeout(timer);
  }, [enabled]);
  return now;
}
