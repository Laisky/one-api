import { useState, useEffect } from "react";

const MOBILE_BREAKPOINT = 768;
const TABLET_BREAKPOINT = 1024;
const DESKTOP_BREAKPOINT = 1280;

interface ViewportSize {
  width: number;
  height: number;
}

function getSafeViewport(): ViewportSize {
  if (typeof window === "undefined") {
    return { width: 0, height: 0 };
  }

  const visualViewport = window.visualViewport;
  const doc = typeof document !== "undefined" ? document.documentElement : null;
  const screen = typeof window.screen !== "undefined" ? window.screen : null;

  const widthCandidates = [window.innerWidth];
  const heightCandidates = [window.innerHeight];

  if (visualViewport) {
    widthCandidates.push(visualViewport.width);
    heightCandidates.push(visualViewport.height);
  }

  if (doc) {
    widthCandidates.push(doc.clientWidth);
    heightCandidates.push(doc.clientHeight);
  }

  if (screen) {
    if (typeof screen.width === "number") {
      widthCandidates.push(screen.width);
    }
    if (typeof screen.height === "number") {
      heightCandidates.push(screen.height);
    }
    if (typeof screen.availWidth === "number") {
      widthCandidates.push(screen.availWidth);
    }
    if (typeof screen.availHeight === "number") {
      heightCandidates.push(screen.availHeight);
    }
  }

  const width = Math.min(
    ...widthCandidates.filter(
      (v): v is number => typeof v === "number" && v > 0
    )
  );
  const height = Math.min(
    ...heightCandidates.filter(
      (v): v is number => typeof v === "number" && v > 0
    )
  );

  return {
    width: Math.round(width),
    height: Math.round(height),
  };
}

interface BreakpointState {
  isMobile: boolean;
  isTablet: boolean;
  isDesktop: boolean;
  isLarge: boolean;
  currentBreakpoint: "mobile" | "tablet" | "desktop" | "large";
  width: number;
  height: number;
}

export function useResponsive(): BreakpointState {
  const [state, setState] = useState<BreakpointState>(() => {
    if (typeof window !== "undefined") {
      const { width, height } = getSafeViewport();
      return {
        width,
        height,
        isMobile: width < MOBILE_BREAKPOINT,
        isTablet: width >= MOBILE_BREAKPOINT && width < TABLET_BREAKPOINT,
        isDesktop: width >= TABLET_BREAKPOINT && width < DESKTOP_BREAKPOINT,
        isLarge: width >= DESKTOP_BREAKPOINT,
        currentBreakpoint:
          width < MOBILE_BREAKPOINT
            ? "mobile"
            : width < TABLET_BREAKPOINT
              ? "tablet"
              : width < DESKTOP_BREAKPOINT
                ? "desktop"
                : "large",
      };
    }

    // Server-side rendering fallback
    return {
      width: 0,
      height: 0,
      isMobile: false,
      isTablet: false,
      isDesktop: true,
      isLarge: false,
      currentBreakpoint: "desktop",
    };
  });

  useEffect(() => {
    const updateState = () => {
      const { width, height } = getSafeViewport();

      setState((prev) => {
        if (prev.width === width && prev.height === height) {
          return prev;
        }

        return {
          width,
          height,
          isMobile: width < MOBILE_BREAKPOINT,
          isTablet: width >= MOBILE_BREAKPOINT && width < TABLET_BREAKPOINT,
          isDesktop: width >= TABLET_BREAKPOINT && width < DESKTOP_BREAKPOINT,
          isLarge: width >= DESKTOP_BREAKPOINT,
          currentBreakpoint:
            width < MOBILE_BREAKPOINT
              ? "mobile"
              : width < TABLET_BREAKPOINT
                ? "tablet"
                : width < DESKTOP_BREAKPOINT
                  ? "desktop"
                  : "large",
        };
      });
    };

    // Update on mount
    updateState();

    // Add event listener with debouncing
    let timeoutId: ReturnType<typeof setTimeout> | undefined;
    const debouncedUpdate = () => {
      if (timeoutId) {
        clearTimeout(timeoutId);
      }
      timeoutId = setTimeout(updateState, 100);
    };

    window.addEventListener("resize", debouncedUpdate);
    window.visualViewport?.addEventListener("resize", debouncedUpdate);
    window.visualViewport?.addEventListener("scroll", debouncedUpdate);

    return () => {
      window.removeEventListener("resize", debouncedUpdate);
      window.visualViewport?.removeEventListener("resize", debouncedUpdate);
      window.visualViewport?.removeEventListener("scroll", debouncedUpdate);
      if (timeoutId) {
        clearTimeout(timeoutId);
      }
    };
  }, []);

  return state;
}

// Additional utility hooks for specific breakpoints
export function useIsMobile(): boolean {
  const { isMobile } = useResponsive();
  return isMobile;
}

export function useIsTablet(): boolean {
  const { isTablet } = useResponsive();
  return isTablet;
}

export function useIsDesktop(): boolean {
  const { isDesktop, isLarge } = useResponsive();
  return isDesktop || isLarge;
}

// Hook for media query matching
export function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState(() => {
    if (typeof window !== "undefined") {
      return window.matchMedia(query).matches;
    }
    return false;
  });

  useEffect(() => {
    const mediaQuery = window.matchMedia(query);
    const handler = (event: MediaQueryListEvent) => {
      setMatches(event.matches);
    };

    mediaQuery.addEventListener("change", handler);
    setMatches(mediaQuery.matches);

    return () => mediaQuery.removeEventListener("change", handler);
  }, [query]);

  return matches;
}

// Predefined media query hooks
export function useIsTouchDevice(): boolean {
  return useMediaQuery("(hover: none) and (pointer: coarse)");
}

export function usePrefersReducedMotion(): boolean {
  return useMediaQuery("(prefers-reduced-motion: reduce)");
}

export function usePrefersDarkMode(): boolean {
  return useMediaQuery("(prefers-color-scheme: dark)");
}

export function useIsLandscape(): boolean {
  return useMediaQuery("(orientation: landscape)");
}

export function useIsPortrait(): boolean {
  return useMediaQuery("(orientation: portrait)");
}
