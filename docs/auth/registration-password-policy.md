# Registration password policy

## Product contract

New password registrations through Modern **and** `POST /api/user/register` use
10–30 characters, at least one ASCII letter (`A–Z` or `a–z`), and at least one
ASCII digit (`0–9`). All visible ASCII punctuation is allowed but not required.
The exact character range is U+0021–U+007E (`!` through `~`). Quotes, slashes and
backslashes are supported, not blacklisted as a substitute for safe encoding.

Reject spaces anywhere, tabs, newlines, other control characters, DEL, non-ASCII
whitespace, accented characters, full-width characters and emoji. Reject invalid
input with guidance; do not trim, normalize, escape into a different password,
truncate, or log it. Normal paste, password-manager autocomplete and the independent
show/hide controls remain available. A checklist match is not a strength estimate.

## Standards and deliberate deviations

[NIST SP 800-63B-4](https://pages.nist.gov/800-63-4/sp800-63b.html#passwordver)
and the [OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html#implement-proper-password-strength-controls)
recommend allowing spaces, avoiding character-mixture mandates, supporting a
maximum of at least 64 characters, and checking new passwords against common or
compromised values. NIST requires a 15-character minimum for single-factor passwords.

The explicitly requested 10–30 range, letter/digit requirement, ASCII-only rule
and whitespace prohibition are **product choices**, not NIST compliance or proof
that spaces are inherently insecure. Do not add a mandatory uppercase/lowercase
mixture, mandatory symbol, or an arbitrary tiny punctuation allowlist. Prefer a
unique randomly generated password of 15+ characters and MFA. This scoped change
does not introduce or claim a comprehensive breached-password screening service,
a hashing migration, or periodic forced password changes.

## Implementation and compatibility

`dto.IsRegistrationPasswordValid` is a pure, bounded validator called by
`controller.Register` after existing request-shape validation and before email
verification, account creation or default-token generation. Modern's live checks
and submission schema share `registration-password.ts`. Go policy tests, actual
registration-handler tests and frontend tests consume the same JSON fixtures in
`web/modern/src/lib/testdata/registration-passwords.json`. Keeping the fixtures
inside `web/` preserves the existing frontend-only Docker build context.

The stricter server-side signup policy intentionally rejects previously accepted
8–9-character, whitespace-containing, non-ASCII, all-letter or all-digit signup
passwords. This applies to direct API calls and other themes using the registration
endpoint too; their existing server-error presentation provides correction guidance.
No request fields or success envelopes change. Existing malformed/type/length
request errors retain their earlier handling.

Do **not** apply this new-signup validator to login, stored hashes, OAuth/passkey
accounts, or a profile-only update. Existing credentials continue to authenticate
as entered. Password resets, admin creation and self-service password changes
retain their existing policies; their 30-character DTO/model maximum expansion
from the earlier PR commit is retained. They are not silently migrated by a
registration UX change. There is no database migration or forced password reset.

## Qualification and delivery

Run the focused tests first; do not repeatedly rebuild every service to iterate
on a password-policy case:

```sh
go test ./dto -run '^TestRegistrationPassword' -count=1
go test ./controller -run '^(TestRegistrationPassword|TestUserPassword|TestRegisterDuplicateUsername)' -count=1
cd web/modern
yarn install --frozen-lockfile
yarn vitest run src/lib/registration-password.test.ts src/pages/auth/__tests__/RegisterPage src/pages/auth/__tests__/LoginPage
yarn type-check
yarn check:i18n
cd ../..
make build-frontend-modern
```

The API tests verify 30-character registration followed by login, unchanged
full-password hashing, invalid signup rejection without user/token writes, email
code correction, and existing short/spaced/non-ASCII credential login. Frontend
tests cover every ASCII character, Unicode/whitespace rejection, live matching,
paste without silent trimming, correction after rejection, and unchanged payloads.
Also inspect mobile/desktop layout and keyboard behavior in a real browser.

The ordinary quick CI does not replace manual React/build qualification. Do not
merge/release while that evidence is missing. Release the backend and rebuilt
Modern together, or the backend first, so no active API still enforces the old
20-character maximum when the UI accepts 30. During rollout and rollback, verify
login with an existing 30-character password. Reverting requires no database
rollback; it restores the prior rules for future signups/updates and never rewrites
or truncates an existing password hash.
