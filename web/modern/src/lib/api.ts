import axios from "axios";

// Unified API client - callers must provide complete URLs including /api prefix
// This eliminates ambiguity and ensures consistency across all API calls
export const api = axios.create({
  timeout: 10000,
});

// Always send cookies for session-based auth
api.defaults.withCredentials = true;

// Request interceptor
api.interceptors.request.use(
  (config) => {
    // For session-based authentication, we rely on cookies (withCredentials: true)
    // Only add Authorization header for specific API endpoints that require token auth
    // Most dashboard/web endpoints use session-based auth via cookies
    const token = localStorage.getItem("token");
    if (token && config.url?.startsWith("/v1/")) {
      // Only add token for API endpoints that require token authentication
      config.headers.Authorization = `Bearer ${token}`;
    }

    // Disable caching for all GET requests to /api endpoints to always fetch fresh data
    if (
      config.method?.toLowerCase() === "get" &&
      config.url &&
      config.url.startsWith("/api")
    ) {
      // Set explicit no-cache headers
      config.headers["Cache-Control"] = "no-cache, no-store, must-revalidate";
      config.headers["Pragma"] = "no-cache";
      config.headers["Expires"] = "0";

      // Append a cache-busting timestamp query param while preserving existing params
      try {
        const urlObj = new URL(config.url, window.location.origin);
        urlObj.searchParams.set("_", Date.now().toString());
        config.url = urlObj.pathname + urlObj.search;
      } catch (_e) {
        // Fallback: if URL constructor fails (should not for relative paths), leave URL unchanged
      }
    }
    return config;
  },
  (error) => Promise.reject(error)
);

// Helper function to handle authentication failures
const handleAuthFailure = () => {
  // Clear auth data
  localStorage.removeItem("token");
  localStorage.removeItem("user");

  // Get current path for redirect_to parameter
  const currentPath = window.location.pathname + window.location.search;
  const redirectTo = encodeURIComponent(currentPath);

  // Redirect to login with redirect_to parameter
  window.location.href = `/login?redirect_to=${redirectTo}`;
};

// Response interceptor
api.interceptors.response.use(
  (response) => {
    // Handle legacy 200 OK with success: false for auth errors
    if (response.data && response.data.success === false) {
      const url = response.config?.url || "";
      const message = (response.data.message || "").toLowerCase();
      const isAuthError =
        message.includes("access token is invalid") ||
        message.includes("not logged in");

      // Do not redirect for known public endpoints
      const isPublicEndpoint = url.startsWith("/api/models/display");

      if (isAuthError && !isPublicEndpoint) {
        handleAuthFailure();
        return response;
      }
    }
    return response;
  },
  (error) => {
    // Handle proper HTTP status codes for authentication/authorization failures
    const status = error.response?.status;
    const message = (error.response?.data?.message || "").toLowerCase();
    if (
      status === 401 ||
      (status === 403 && message.includes("not logged in"))
    ) {
      handleAuthFailure();
    }
    return Promise.reject(error);
  }
);

export default api;
