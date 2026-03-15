# Dashboard Product Requirements Document

## Overview

The Dashboard provides a comprehensive, interactive overview of system usage, user activity, and key metrics. It is designed for both root and regular users, with dynamic data visualization, filtering, and actionable insights. This document details all functional requirements necessary to replicate the current dashboard implementation.

---

## 1. User Roles & Access

- **Root User:**
  - Can view and select all users from a dropdown to filter dashboard data by user.
  - Has access to user-specific and global statistics.
- **Regular User:**
  - Sees only their own data; user selection is not available.

---

## 2. Date Range Selection & Validation

- **Custom Date Range:**
  - Users can select `fromDate` and `toDate` for data filtering.
  - Date pickers must validate that `fromDate` ≤ `toDate`.
  - If invalid, display a clear error message and disable data refresh.
- **Preset Ranges:**
  - Quick-select options for common ranges (e.g., Today, Last 7 Days, Last 30 Days).
  - Selecting a preset updates the date fields and triggers data reload.

---

## 3. Data Fetching & Refresh

- **Initial Load:**
  - On first render, fetches user list (if root) and dashboard data for the default date range.
- **Manual Refresh:**
  - A refresh button triggers data reload for the current filters.
  - Show a loading indicator during fetch.
- **Last Updated:**
  - Display the timestamp of the last successful data fetch.

---

## 4. Summary Metrics (Top Section)

Display the following summary statistics, calculated for the selected date range and user:

- Today’s Requests
- Today’s Quota Usage
- Today’s Token Usage
- Average Cost per Request
- Average Tokens per Request
- Top Model (by usage)
- Total Models Used
- Request Trend (vs previous period)
- Quota Trend (vs previous period)
- Token Trend (vs previous period)
- Average Response Time
- Success Rate (%)
- Throughput (requests/sec)

---

## 5. Data Visualization

### 5.1 Time Series Line Chart

- **Metrics:**
  - Requests, Quota, Tokens (toggle/selectable metric)
- **Features:**
  - X-axis: Date (formatted, with all dates in range, including those with zero data)
  - Y-axis: Metric value
  - Smooth lines, no dots except for active points
  - Custom gradients for each metric
  - Responsive, with styled background and rounded corners
  - Grid lines: horizontal only, semi-transparent
  - Tooltip on hover

### 5.2 Stacked Bar Chart (Model Usage)

- **Metrics:**
  - Requests per model (stacked by model)
- **Features:**
  - Each model assigned a unique color (from a defined palette)
  - X-axis: Date
  - Y-axis: Requests
  - Legend for model names
  - Responsive and styled as above

---

## 6. Model Analytics

- **Unique Models List:**
  - List all unique models used in the period, sorted by request count (descending).
- **Model Efficiency:**
  - Calculate and display efficiency metrics per model (e.g., tokens/request, cost/request).
- **Usage Patterns:**
  - Analyze and display peak usage times and trends.
- **Cost Optimization Insights:**
  - Generate and display recommendations for reducing cost based on usage patterns.

---

## 7. UI/UX Details

- **Loading State:**
  - Show skeleton loaders for charts and summary while data is loading.
- **Error Handling:**
  - Display clear error messages for invalid date ranges or failed data fetches.
- **Responsive Design:**
  - Layout and charts adapt to different screen sizes.
- **Styling:**
  - Use transparent backgrounds, rounded corners, and consistent color themes.
  - All chart elements (lines, bars, gradients) use the defined color palette.
- **Accessibility:**
  - All interactive elements are keyboard accessible.
  - Charts use sufficient color contrast and tooltips.

---

## 8. Technical/Implementation Details

- **Chart Configuration:**
  - Centralized config for chart styles, colors, gradients, and bar colors.
- **Date Formatting:**
  - All dates displayed in a consistent, user-friendly format.
- **State Management:**
  - Use React state and hooks for all data, filters, and UI state.
- **Performance:**
  - Minimize unnecessary re-renders and data fetches.
- **Extensibility:**
  - All helper functions (data processing, analytics) are modular and reusable.

---

## 9. Non-Functional Requirements

- **Localization:**
  - All user-facing text supports i18n (translation-ready).
- **Security:**
  - Only authorized users can access dashboard data.
- **Maintainability:**
  - Code is modular, well-documented, and follows project conventions.

---

## 10. Out of Scope

- No direct data export or download features.
- No real-time auto-refresh (manual refresh only).

---

## 11. Acceptance Criteria

- All functional points above are implemented and visually match the current dashboard.
- All metrics, charts, and analytics are accurate for any user and date range.
- UI is responsive, accessible, and matches the defined style.
- All error and loading states are handled gracefully.

---

## 12. References

- See `web/modern/src/pages/Dashboard/` for implementation details.
- Chart color palette and gradients are defined in the `chartConfig` object.

---
