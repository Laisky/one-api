import { useEffect, useMemo, useRef, useState } from 'react';
import { resolveTimeWindowSnapshot, timeWindowScheduleSignature, type TimeWindowSchedule, type TimeWindowSnapshot } from './time-window';

/**
 * useTimeWindowSnapshot tracks active/history window membership from one instant,
 * re-evaluating it every second and cleaning up its timer on unmount.
 * Parameters: windows is the ordered window list of the model being displayed.
 * Returns: the live snapshot; one call per pricing view keeps the current tariff
 * and the window cards on the same active index.
 */
export function useTimeWindowSnapshot(windows: TimeWindowSchedule[] | undefined): TimeWindowSnapshot {
  const windowsRef = useRef(windows);
  windowsRef.current = windows;
  const signature = useMemo(() => timeWindowScheduleSignature(windows), [windows]);
  const [snapshot, setSnapshot] = useState(() => ({ signature, ...resolveTimeWindowSnapshot(windows, new Date()) }));

  useEffect(() => {
    // A model without windows has nothing to re-evaluate.
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

/**
 * usePricingMinute returns the current instant and advances it at every
 * wall-clock minute while enabled, so a list that quotes scheduled tariffs
 * re-selects them when a minute-granular window starts or ends.
 * Parameters: enabled turns the minute timer on (any model has time windows).
 * Returns: the instant the caller should select tariffs at.
 */
export function usePricingMinute(enabled: boolean): Date {
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
