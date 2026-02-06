## 2025-05-22 - [Inconsistent Delete Confirmation & Missing ARIA Labels]
**Learning:** Inconsistent interaction patterns (like some tables having delete confirmation and others not) can lead to accidental data loss and user frustration. Icon-only buttons without ARIA labels make the interface inaccessible to screen reader users.
**Action:** Always ensure destructive actions have a confirmation step and all icon-only buttons have descriptive ARIA labels. Check for existing patterns in the codebase to ensure consistency.
