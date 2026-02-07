## 2025-05-22 - [Inconsistent Delete Confirmation & Missing ARIA Labels]
**Learning:** Inconsistent interaction patterns (like some tables having delete confirmation and others not) can lead to accidental data loss and user frustration. Icon-only buttons without ARIA labels make the interface inaccessible to screen reader users.
**Action:** Always ensure destructive actions have a confirmation step and all icon-only buttons have descriptive ARIA labels. Check for existing patterns in the codebase to ensure consistency.

## 2025-05-22 - [Package Manager Consistency & i18n Best Practices]
**Learning:** Mixing package managers (like using pnpm in a yarn project) can lead to environment issues and unexpected lockfile changes. Hardcoding English fallbacks in translation calls (`t('key', 'Fallback')`) bypasses the i18n system and leads to inconsistent localized experiences.
**Action:** Always verify the project's preferred package manager before installing dependencies. Ensure all new UI strings are properly added to all supported locale files instead of using hardcoded fallbacks.

## 2025-05-22 - [Package Manager Consistency & i18n Best Practices]
**Learning:** Mixing package managers (like using pnpm in a yarn project) can lead to environment issues and unexpected lockfile changes. Hardcoding English fallbacks in translation calls (`t('key', 'Fallback')`) bypasses the i18n system and leads to inconsistent localized experiences.
**Action:** Always verify the project's preferred package manager before installing dependencies. Ensure all new UI strings are properly added to all supported locale files instead of using hardcoded fallbacks.
