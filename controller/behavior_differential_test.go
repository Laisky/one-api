package controller

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
)

// The controller/behavior_differential*_test.go files implement the T17/T18
// black-box differential harness described in
// docs/proposals/archive/20260714_boundary-response-dtos.md. It drives every handler
// site in the proposal's Appendix A (28 sites: User 6, Token 9, Channel 4,
// Redemption 4, Log 5) plus the documented T18 error cases through the real
// handlers over httptest, and records (status, canonicalized JSON body) into
// controller/testdata/behavior/*.json.
//
// The baselines were captured on the PRE-refactor tree (git 6ebe28a5, which is
// the last commit before this refactor) and are replayed on the POST-refactor
// tree: a zero diff is the mechanical proof that relocating the external JSON
// contract from model-level MarshalJSON methods to boundary dto.*Response
// mappers changed no observable behavior. 74 of 81 cases are such a zero diff;
// the 7 exceptions are recorded in behaviorDiffKnownDeviations below.
//
// The harness files deliberately reference only handler funcs, model entity
// types/constants, common/* helpers, gin, httptest, testify and the stdlib, and
// must never mention dto.*Response, ToResponse or XsToResponses. That constraint
// is what allowed these exact files (a single file at capture time, since split
// by entity) to be compiled and run inside a pre-refactor worktree to generate
// the baselines in the first place; keeping it means the capture can be
// reproduced from scratch rather than taken on trust.
//
// Caveat when replaying in a pre-refactor worktree: the 7 cases listed in
// behaviorDiffKnownDeviations assert POST-refactor expectations and will fail
// there, correctly reporting that the output matches the pre-refactor baseline.
// The other 74 pass on both trees.
//
// Regenerate the baselines with:
//
//	go test ./controller/ -run TestBehaviorDifferential -count=1 -update-behavior
//
// Regeneration is a contract change and must be reviewed as such. In particular,
// regenerating on the CURRENT tree silently replaces pre-refactor truth with
// whatever the code does today, which would make this harness tautological — to
// re-record an accepted difference, use -update-behavior-deviations instead,
// which never touches the baselines.

// behaviorDiffUpdate regenerates controller/testdata/behavior/*.json instead of
// comparing against them. Pass -update-behavior to enable.
var behaviorDiffUpdate = flag.Bool("update-behavior", false,
	"regenerate the T17 behavior baselines in controller/testdata/behavior instead of comparing against them")

// behaviorDiffUpdateDeviations regenerates ONLY the known-deviation
// expectations in controller/testdata/behavior/deviations. It never touches the
// pre-refactor baselines, so the historical record cannot be lost by running it.
var behaviorDiffUpdateDeviations = flag.Bool("update-behavior-deviations", false,
	"regenerate the post-refactor expectations for cases listed in behaviorDiffKnownDeviations")

const behaviorDiffBaselineDir = "testdata/behavior"

// behaviorDiffDeviationDir holds the POST-refactor expectation for the small,
// explicitly reviewed set of cases whose behavior intentionally changed. The
// pre-refactor bytes stay in behaviorDiffBaselineDir as the historical record,
// so both sides of every accepted deviation remain readable in the tree.
const behaviorDiffDeviationDir = behaviorDiffBaselineDir + "/deviations"

// behaviorDiffKnownDeviations enumerates the cases where post-refactor behavior
// intentionally differs from the pre-refactor baseline, mapped to the reason on
// record. See §10.3 of docs/proposals/archive/20260714_boundary-response-dtos.md.
//
// These are NOT skips. A listed case is still asserted byte-for-byte against its
// recorded post-refactor expectation, and it is additionally required to still
// differ from the pre-refactor baseline — so a stale entry cannot sit here
// quietly masking a case that has since converged back.
//
// Adding an entry here is a deliberate contract change and must be reviewed as
// such: it is the one escape hatch in this harness, and it exists so that an
// accepted deviation is recorded in code rather than hidden by re-baselining.
var behaviorDiffKnownDeviations = map[string]string{
	"user/register_wrong_type_unbound_field":    behaviorDiffUnboundFieldReason,
	"user/create_wrong_type_unbound_field":      behaviorDiffUnboundFieldReason,
	"user/update_self_wrong_type_unbound_field": behaviorDiffUnboundFieldReason,
	"token/update_missing_uuid":                 behaviorDiffMissingUUIDStatusReason,
	"channel/update_missing_uuid":               behaviorDiffMissingUUIDStatusReason,
	"redemption/update_missing_uuid":            behaviorDiffMissingUUIDStatusReason,
	"token/admin_get_by_uuid_not_found":         behaviorDiffAdminTokenNotFoundReason,
}

// behaviorDiffAdminTokenNotFoundReason documents the accepted message change
// for AdminGetToken when a well-formed UUID matches no token.
const behaviorDiffAdminTokenNotFoundReason = "" +
	"AdminGetToken prefixed every resolution failure with 'invalid token id: ', " +
	"so a well-formed UUID that matches no token answered 'invalid token id: " +
	"resource reference not found' and a database outage answered 'invalid " +
	"token id: resolve uuid reference: ...', both telling the caller the id was " +
	"malformed when it was not. Only a malformed id keeps the prefix now; a " +
	"missing token answers the shared 'resource reference not found' that " +
	"GetToken and every other resolver site already return. Status (200) and " +
	"log level (WARN, not_found) are unchanged. No bundled theme or CLI matches " +
	"this message."

// behaviorDiffMissingUUIDStatusReason documents the accepted status change for
// strict-in update bodies that omit uuid.
const behaviorDiffMissingUUIDStatusReason = "" +
	"An update body without uuid used to answer HTTP 200 with an unclassified " +
	"error, which the shared handler logger recorded as 'http handler error' at " +
	"ERROR with a stack trace, paging on-call for a client mistake. A missing or " +
	"malformed resource reference is now an invalid_request client error that " +
	"answers HTTP 400 and logs at WARN without a stack. The body and message are " +
	"unchanged. No bundled theme or CLI sends an update without uuid."

// behaviorDiffUnboundFieldReason documents the single accepted I2 deviation.
const behaviorDiffUnboundFieldReason = "" +
	"Register/CreateUser/UpdateSelf used to decode into model.User, so a type " +
	"mismatch on ANY of its JSON-tagged fields — including fields the handler " +
	"never read — rejected the whole body with 400 'invalid parameter'. The " +
	"narrow request DTOs omit those fields, so encoding/json now discards them " +
	"as unknown and the request succeeds. The old strictness was an accident of " +
	"decoding into the entity, not a designed check. Restoring it exactly would " +
	"require each DTO to mirror model.User's entire pre-refactor tag set, " +
	"including the four secret fields whose tags were deliberately removed for " +
	"G3 — re-creating the coupling this refactor removes. The relaxation only " +
	"loosens validation on fields these handlers ignore: no secret, no id, no " +
	"stored row changes, and no bundled frontend can trigger it (all three " +
	"themes send such keys only with correctly-typed values echoed from the " +
	"server's own encoding). Accepted and documented rather than re-baselined."

// behaviorDiffRequest describes one HTTP call against a real handler.
type behaviorDiffRequest struct {
	method   string
	pattern  string
	target   func(f *behaviorDiffFixture) string
	body     func(f *behaviorDiffFixture) string
	prepare  func(c *gin.Context, f *behaviorDiffFixture)
	handler  gin.HandlerFunc
	sessions bool
}

// behaviorDiffCase is a single captured (status, canonical body) observation.
type behaviorDiffCase struct {
	entity  string
	name    string
	site    string
	req     behaviorDiffRequest
	dynamic func(t *testing.T, f *behaviorDiffFixture, n *behaviorDiffNormalizer)
}

// behaviorDiffRecord is the on-disk baseline shape.
type behaviorDiffRecord struct {
	Case   string          `json:"case"`
	Site   string          `json:"site"`
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

// behaviorDiffServe runs one request through a real handler on a bare gin engine.
// Parameters:
//   - t: active test handle.
//   - f: seeded fixture the request is issued against.
//   - r: request description (route pattern, target, body, context preparation).
//
// Return values:
//   - *httptest.ResponseRecorder: the recorded handler response.
func behaviorDiffServe(t *testing.T, f *behaviorDiffFixture, r behaviorDiffRequest) *httptest.ResponseRecorder {
	t.Helper()

	engine := gin.New()
	if r.sessions {
		engine.Use(sessions.Sessions("behaviordiff-session", cookie.NewStore([]byte("behaviordiff-secret"))))
	}
	engine.Use(func(c *gin.Context) {
		gmw.SetLogger(c, logger.Logger)
		c.Set(helper.RequestIdKey, behaviorDiffRequestID)
		if r.prepare != nil {
			r.prepare(c, f)
		}
	})
	engine.Handle(r.method, r.pattern, r.handler)

	var reader *strings.Reader
	if r.body != nil {
		reader = strings.NewReader(r.body(f))
	} else {
		reader = strings.NewReader("")
	}
	request := httptest.NewRequest(r.method, r.target(f), reader)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	f.requestStart = helper.GetTimestamp()
	engine.ServeHTTP(recorder, request)
	f.requestEnd = helper.GetTimestamp()
	return recorder
}

// behaviorDiffEncode serializes v deterministically: map keys are sorted by
// encoding/json and HTML escaping is disabled so the "<placeholder>" tokens stay
// readable in the committed baselines.
// Parameters:
//   - t: active test handle.
//   - v: value to encode.
//   - indent: indentation applied to nested values.
//
// Return values:
//   - []byte: encoded JSON without the encoder's trailing newline.
func behaviorDiffEncode(t *testing.T, v any, indent string) []byte {
	t.Helper()
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", indent)
	require.NoError(t, encoder.Encode(v))
	return bytes.TrimRight(buf.Bytes(), "\n")
}

// behaviorDiffCanonicalize decodes a response body, rewrites known fixture values
// to placeholders and re-encodes deterministically (map keys sorted by
// encoding/json) so byte comparison across trees is stable.
// Parameters:
//   - t: active test handle.
//   - raw: raw response bytes.
//   - n: normalizer primed from the fixture.
//
// Return values:
//   - json.RawMessage: canonical body, or nil for an empty response.
func behaviorDiffCanonicalize(t *testing.T, raw []byte, n *behaviorDiffNormalizer) json.RawMessage {
	t.Helper()
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var decoded any
	require.NoError(t, decoder.Decode(&decoded), "response body must be valid JSON: %s", raw)

	return json.RawMessage(behaviorDiffEncode(t, n.apply(decoded), ""))
}

// TestBehaviorDifferential drives every Appendix A handler site plus its T18
// error cases and byte-compares (status, canonical body) against the baselines
// captured on the pre-refactor tree.
// Parameters:
//   - t: active test handle.
//
// Return values: none.
func TestBehaviorDifferential(t *testing.T) {
	gin.SetMode(gin.TestMode)

	if *behaviorDiffUpdate {
		require.NoError(t, os.MkdirAll(behaviorDiffBaselineDir, 0o755))
	}

	for _, tc := range behaviorDiffCases() {
		name := tc.entity + "/" + tc.name
		t.Run(name, func(t *testing.T) {
			fixture := behaviorDiffSetup(t)
			recorder := behaviorDiffServe(t, fixture, tc.req)
			if tc.dynamic != nil {
				tc.dynamic(t, fixture, fixture.norm)
			}

			record := behaviorDiffRecord{
				Case:   name,
				Site:   tc.site,
				Status: recorder.Code,
				Body:   behaviorDiffCanonicalize(t, recorder.Body.Bytes(), fixture.norm),
			}
			got := append(behaviorDiffEncode(t, record, "  "), '\n')

			path := filepath.Join(behaviorDiffBaselineDir, tc.entity+"_"+tc.name+".json")
			if *behaviorDiffUpdate {
				require.NoError(t, os.WriteFile(path, got, 0o644))
				return
			}

			baseline, err := os.ReadFile(path)
			require.NoError(t, err, "missing behavior baseline %s; regenerate with -update-behavior", path)

			reason, deviates := behaviorDiffKnownDeviations[name]
			if !deviates {
				require.Equal(t, string(baseline), string(got),
					"observable behavior changed for %s (%s); this harness exists to catch exactly this", name, tc.site)
				return
			}

			// Accepted deviation: assert against the recorded post-refactor bytes
			// instead of the pre-refactor baseline, but keep asserting.
			devPath := filepath.Join(behaviorDiffDeviationDir, tc.entity+"_"+tc.name+".json")
			if *behaviorDiffUpdateDeviations {
				require.NoError(t, os.MkdirAll(behaviorDiffDeviationDir, 0o755))
				require.NoError(t, os.WriteFile(devPath, got, 0o644))
				return
			}
			want, err := os.ReadFile(devPath)
			require.NoError(t, err,
				"missing deviation expectation %s; regenerate with -update-behavior-deviations", devPath)
			require.Equal(t, string(want), string(got),
				"behavior drifted for known-deviation case %s (%s); reason on record: %s", name, tc.site, reason)
			require.NotEqual(t, string(baseline), string(got),
				"case %s is listed in behaviorDiffKnownDeviations but now matches the pre-refactor "+
					"baseline; delete the entry and its deviation file rather than leave it masking the case", name)
		})
	}
}

// behaviorDiffAsUser sets the self-scoped auth context keys for the fixture user.
// Parameters:
//   - c: gin request context.
//   - f: seeded fixture.
//
// Return values: none.
func behaviorDiffAsUser(c *gin.Context, f *behaviorDiffFixture) {
	c.Set(ctxkey.Id, f.user.Id)
	c.Set(ctxkey.UserUUID, f.user.UUID)
	c.Set(ctxkey.Role, model.RoleCommonUser)
}

// behaviorDiffAsRoot sets the root-admin auth context keys.
// Parameters:
//   - c: gin request context.
//   - f: seeded fixture.
//
// Return values: none.
func behaviorDiffAsRoot(c *gin.Context, f *behaviorDiffFixture) {
	c.Set(ctxkey.Id, f.root.Id)
	c.Set(ctxkey.UserUUID, f.root.UUID)
	c.Set(ctxkey.Role, model.RoleRootUser)
}

// behaviorDiffAsAdmin sets the (non-root) admin auth context keys.
// Parameters:
//   - c: gin request context.
//   - f: seeded fixture.
//
// Return values: none.
func behaviorDiffAsAdmin(c *gin.Context, f *behaviorDiffFixture) {
	c.Set(ctxkey.Id, f.other.Id)
	c.Set(ctxkey.UserUUID, f.other.UUID)
	c.Set(ctxkey.Role, model.RoleAdminUser)
}

// behaviorDiffStatic returns a target/body builder for a value that does not
// depend on the fixture.
// Parameters:
//   - value: literal URL or body.
//
// Return values:
//   - func(*behaviorDiffFixture) string: builder returning value verbatim.
func behaviorDiffStatic(value string) func(f *behaviorDiffFixture) string {
	return func(*behaviorDiffFixture) string { return value }
}

// behaviorDiffMissingUUID is a well-formed UUID that matches no fixture row; it
// drives the "non-existent uuid" error cases.
const behaviorDiffMissingUUID = "018f0000-0000-7000-8000-0000deadbeef"

// behaviorDiffCases enumerates every Appendix A site (User 6, Token 9,
// Channel 4, Redemption 4, Log 5) plus the T18 error cases.
// Parameters: none.
//
// Return values:
//   - []behaviorDiffCase: the full capture set.
func behaviorDiffCases() []behaviorDiffCase {
	return append(append(append(append(
		behaviorDiffUserCases(),
		behaviorDiffTokenCases()...),
		behaviorDiffChannelCases()...),
		behaviorDiffRedemptionCases()...),
		behaviorDiffLogCases()...)
}
