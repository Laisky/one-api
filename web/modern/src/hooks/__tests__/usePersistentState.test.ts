import { renderHook, act } from "@testing-library/react";
import { vi, describe, it, expect, beforeEach, afterEach } from "vitest";
import {
  usePersistentState,
  usePageSize,
  getStoredPageSize,
  STORAGE_KEYS,
} from "../usePersistentState";

describe("usePersistentState", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  afterEach(() => {
    localStorage.clear();
  });

  it("should return default value when no stored value exists", () => {
    const { result } = renderHook(() =>
      usePersistentState("test-key", "default-value")
    );
    expect(result.current[0]).toBe("default-value");
  });

  it("should return stored value when it exists", () => {
    localStorage.setItem("test-key", JSON.stringify("stored-value"));
    const { result } = renderHook(() =>
      usePersistentState("test-key", "default-value")
    );
    expect(result.current[0]).toBe("stored-value");
  });

  it("should persist value to localStorage when updated", () => {
    const { result } = renderHook(() =>
      usePersistentState("test-key", "default-value")
    );

    act(() => {
      result.current[1]("new-value");
    });

    expect(result.current[0]).toBe("new-value");
    expect(JSON.parse(localStorage.getItem("test-key")!)).toBe("new-value");
  });

  it("should handle updater function", () => {
    const { result } = renderHook(() => usePersistentState("test-key", 10));

    act(() => {
      result.current[1]((prev) => prev + 5);
    });

    expect(result.current[0]).toBe(15);
  });

  it("should handle numeric values", () => {
    localStorage.setItem("test-key", JSON.stringify(42));
    const { result } = renderHook(() => usePersistentState("test-key", 0));
    expect(result.current[0]).toBe(42);
  });

  it("should handle invalid JSON gracefully", () => {
    localStorage.setItem("test-key", "invalid-json{");
    const consoleSpy = vi.spyOn(console, "warn").mockImplementation(() => {});

    const { result } = renderHook(() =>
      usePersistentState("test-key", "default-value")
    );

    expect(result.current[0]).toBe("default-value");
    expect(consoleSpy).toHaveBeenCalled();

    consoleSpy.mockRestore();
  });
});

describe("usePageSize", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  afterEach(() => {
    localStorage.clear();
  });

  it("should return default page size when no stored value exists", () => {
    const { result } = renderHook(() =>
      usePageSize(STORAGE_KEYS.PAGE_SIZE)
    );
    expect(result.current[0]).toBe(10);
  });

  it("should return custom default page size", () => {
    const { result } = renderHook(() =>
      usePageSize(STORAGE_KEYS.PAGE_SIZE, 20, [10, 20, 50, 100])
    );
    expect(result.current[0]).toBe(20);
  });

  it("should persist page size to localStorage", () => {
    const { result } = renderHook(() =>
      usePageSize(STORAGE_KEYS.PAGE_SIZE)
    );

    act(() => {
      result.current[1](20);
    });

    expect(result.current[0]).toBe(20);
    expect(JSON.parse(localStorage.getItem(STORAGE_KEYS.PAGE_SIZE)!)).toBe(
      20
    );
  });

  it("should snap to nearest valid size if invalid size is provided", () => {
    const { result } = renderHook(() =>
      usePageSize(STORAGE_KEYS.PAGE_SIZE, 10, [10, 20, 50, 100])
    );

    act(() => {
      result.current[1](25); // 25 is not in valid sizes, should snap to 20
    });

    expect(result.current[0]).toBe(20);
  });

  it("should handle exact valid sizes", () => {
    const { result } = renderHook(() =>
      usePageSize(STORAGE_KEYS.PAGE_SIZE, 10, [10, 20, 50, 100])
    );

    act(() => {
      result.current[1](50);
    });

    expect(result.current[0]).toBe(50);
  });

  it("should clamp values within bounds", () => {
    const { result } = renderHook(() =>
      usePageSize(STORAGE_KEYS.PAGE_SIZE, 10, [10, 20, 50, 100])
    );

    act(() => {
      result.current[1](5000); // Should clamp to 1000 max, then snap to nearest valid (100)
    });

    expect(result.current[0]).toBe(100);
  });
});

describe("getStoredPageSize", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  afterEach(() => {
    localStorage.clear();
  });

  it("should return default value when no stored value exists", () => {
    expect(getStoredPageSize("non-existent-key", 10)).toBe(10);
  });

  it("should return stored value when it exists", () => {
    localStorage.setItem("test-key", JSON.stringify(25));
    expect(getStoredPageSize("test-key", 10)).toBe(25);
  });

  it("should return default for invalid JSON", () => {
    localStorage.setItem("test-key", "invalid");
    const consoleSpy = vi.spyOn(console, "warn").mockImplementation(() => {});

    expect(getStoredPageSize("test-key", 10)).toBe(10);

    consoleSpy.mockRestore();
  });

  it("should return default for non-positive values", () => {
    localStorage.setItem("test-key", JSON.stringify(-5));
    expect(getStoredPageSize("test-key", 10)).toBe(10);

    localStorage.setItem("test-key", JSON.stringify(0));
    expect(getStoredPageSize("test-key", 10)).toBe(10);
  });
});

describe("STORAGE_KEYS", () => {
  it("should have unique values for all keys", () => {
    const values = Object.values(STORAGE_KEYS);
    const uniqueValues = new Set(values);
    expect(uniqueValues.size).toBe(values.length);
  });

  it("should have correct prefix for all keys", () => {
    Object.values(STORAGE_KEYS).forEach((key) => {
      expect(key).toMatch(/^oneapi_/);
    });
  });
});
