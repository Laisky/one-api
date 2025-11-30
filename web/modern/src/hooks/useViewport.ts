import { useState, useEffect, useCallback } from "react";

interface ViewportState {
  width: number;
  height: number;
  scrollX: number;
  scrollY: number;
  orientation: "portrait" | "landscape";
  devicePixelRatio: number;
}

export function useViewport(): ViewportState {
  const [viewport, setViewport] = useState<ViewportState>(() => {
    if (typeof window === "undefined") {
      return {
        width: 0,
        height: 0,
        scrollX: 0,
        scrollY: 0,
        orientation: "portrait",
        devicePixelRatio: 1,
      };
    }

    return {
      width: window.innerWidth,
      height: window.innerHeight,
      scrollX: window.scrollX,
      scrollY: window.scrollY,
      orientation:
        window.innerWidth > window.innerHeight ? "landscape" : "portrait",
      devicePixelRatio: window.devicePixelRatio || 1,
    };
  });

  const updateViewport = useCallback(() => {
    setViewport({
      width: window.innerWidth,
      height: window.innerHeight,
      scrollX: window.scrollX,
      scrollY: window.scrollY,
      orientation:
        window.innerWidth > window.innerHeight ? "landscape" : "portrait",
      devicePixelRatio: window.devicePixelRatio || 1,
    });
  }, []);

  useEffect(() => {
    let timeoutId: NodeJS.Timeout;

    const debouncedUpdate = () => {
      clearTimeout(timeoutId);
      timeoutId = setTimeout(updateViewport, 100);
    };

    window.addEventListener("resize", debouncedUpdate);
    window.addEventListener("scroll", debouncedUpdate);
    window.addEventListener("orientationchange", debouncedUpdate);

    return () => {
      window.removeEventListener("resize", debouncedUpdate);
      window.removeEventListener("scroll", debouncedUpdate);
      window.removeEventListener("orientationchange", debouncedUpdate);
      clearTimeout(timeoutId);
    };
  }, [updateViewport]);

  return viewport;
}

// Hook for detecting viewport changes
export function useViewportChange(callback: (viewport: ViewportState) => void) {
  const viewport = useViewport();

  useEffect(() => {
    callback(viewport);
  }, [viewport, callback]);
}

// Hook for detecting when viewport crosses specific breakpoints
export function useBreakpointChange(
  breakpoints: Record<string, number>,
  callback: (currentBreakpoint: string, previousBreakpoint: string) => void
) {
  const { width } = useViewport();
  const [currentBreakpoint, setCurrentBreakpoint] = useState<string>("");

  useEffect(() => {
    const sortedBreakpoints = Object.entries(breakpoints).sort(
      ([, a], [, b]) => a - b
    );

    let newBreakpoint = sortedBreakpoints[0][0];

    for (const [name, minWidth] of sortedBreakpoints) {
      if (width >= minWidth) {
        newBreakpoint = name;
      }
    }

    if (newBreakpoint !== currentBreakpoint) {
      const previousBreakpoint = currentBreakpoint;
      setCurrentBreakpoint(newBreakpoint);
      if (previousBreakpoint) {
        callback(newBreakpoint, previousBreakpoint);
      }
    }
  }, [width, breakpoints, currentBreakpoint, callback]);

  return currentBreakpoint;
}

// Hook for safe area insets (for mobile devices with notches)
export function useSafeAreaInsets() {
  const [insets, setInsets] = useState({
    top: 0,
    right: 0,
    bottom: 0,
    left: 0,
  });

  useEffect(() => {
    const updateInsets = () => {
      const style = getComputedStyle(document.documentElement);
      setInsets({
        top: parseInt(
          style.getPropertyValue("env(safe-area-inset-top)") || "0"
        ),
        right: parseInt(
          style.getPropertyValue("env(safe-area-inset-right)") || "0"
        ),
        bottom: parseInt(
          style.getPropertyValue("env(safe-area-inset-bottom)") || "0"
        ),
        left: parseInt(
          style.getPropertyValue("env(safe-area-inset-left)") || "0"
        ),
      });
    };

    updateInsets();
    window.addEventListener("resize", updateInsets);
    window.addEventListener("orientationchange", updateInsets);

    return () => {
      window.removeEventListener("resize", updateInsets);
      window.removeEventListener("orientationchange", updateInsets);
    };
  }, []);

  return insets;
}

// Hook for detecting device capabilities
export function useDeviceCapabilities() {
  const [capabilities, setCapabilities] = useState({
    hasTouch: false,
    hasHover: false,
    hasPointerFine: false,
    hasPointerCoarse: false,
    prefersReducedMotion: false,
    prefersColorScheme: "light" as "light" | "dark",
    prefersContrast: "normal" as "normal" | "high",
  });

  useEffect(() => {
    const updateCapabilities = () => {
      setCapabilities({
        hasTouch: "ontouchstart" in window || navigator.maxTouchPoints > 0,
        hasHover: window.matchMedia("(hover: hover)").matches,
        hasPointerFine: window.matchMedia("(pointer: fine)").matches,
        hasPointerCoarse: window.matchMedia("(pointer: coarse)").matches,
        prefersReducedMotion: window.matchMedia(
          "(prefers-reduced-motion: reduce)"
        ).matches,
        prefersColorScheme: window.matchMedia("(prefers-color-scheme: dark)")
          .matches
          ? "dark"
          : "light",
        prefersContrast: window.matchMedia("(prefers-contrast: high)").matches
          ? "high"
          : "normal",
      });
    };

    updateCapabilities();

    // Listen for changes in media queries
    const mediaQueries = [
      "(hover: hover)",
      "(pointer: fine)",
      "(pointer: coarse)",
      "(prefers-reduced-motion: reduce)",
      "(prefers-color-scheme: dark)",
      "(prefers-contrast: high)",
    ];

    const listeners = mediaQueries.map((query) => {
      const mq = window.matchMedia(query);
      mq.addEventListener("change", updateCapabilities);
      return { mq, handler: updateCapabilities };
    });

    return () => {
      listeners.forEach(({ mq, handler }) => {
        mq.removeEventListener("change", handler);
      });
    };
  }, []);

  return capabilities;
}

// Hook for element size observation
export function useElementSize<T extends HTMLElement>() {
  const [size, setSize] = useState({ width: 0, height: 0 });
  const [elementRef, setElementRef] = useState<T | null>(null);

  useEffect(() => {
    if (!elementRef) return;

    const resizeObserver = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const { width, height } = entry.contentRect;
        setSize({ width, height });
      }
    });

    resizeObserver.observe(elementRef);

    return () => {
      resizeObserver.disconnect();
    };
  }, [elementRef]);

  return [setElementRef, size] as const;
}

// Hook for container queries
export function useContainerQuery(query: string) {
  const [matches, setMatches] = useState(false);
  const [elementRef, setElementRef] = useState<HTMLElement | null>(null);

  useEffect(() => {
    if (!elementRef || !("ResizeObserver" in window)) return;

    const resizeObserver = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const { width, height } = entry.contentRect;

        // Simple container query parsing (extend as needed)
        const match = query.match(/\(min-width:\s*(\d+)px\)/);
        if (match) {
          const minWidth = parseInt(match[1]);
          setMatches(width >= minWidth);
        }
      }
    });

    resizeObserver.observe(elementRef);

    return () => {
      resizeObserver.disconnect();
    };
  }, [elementRef, query]);

  return [setElementRef, matches] as const;
}

// Hook for intersection observer
export function useIntersectionObserver(
  options: IntersectionObserverInit = {}
) {
  const [isIntersecting, setIsIntersecting] = useState(false);
  const [elementRef, setElementRef] = useState<HTMLElement | null>(null);

  useEffect(() => {
    if (!elementRef) return;

    const observer = new IntersectionObserver(([entry]) => {
      setIsIntersecting(entry.isIntersecting);
    }, options);

    observer.observe(elementRef);

    return () => {
      observer.disconnect();
    };
  }, [elementRef, options]);

  return [setElementRef, isIntersecting] as const;
}
