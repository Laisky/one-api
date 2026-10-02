"""Review fixture refinements applied before taking the final validation snapshot."""
from pathlib import Path


def adjust(pr: int, root: Path) -> None:
    """Apply reviewed compatibility and fixture corrections, never altering assertions to hide a leak."""
    if pr == 441:
        path = root / "controller/realtime.go"
        text = path.read_text().replace(
            "use the metered /v1/realtime WebSocket endpoint",
            "use the metered /v1/realtime WebSocket endpoint with a one-api bearer token",
        )
        path.write_text(text)
        # The former provider-specific rejection is now a uniform disabled policy.
        path = root / "controller/gemini_live_review_test.go"
        text = path.read_text().replace("http.StatusBadRequest, w.Code", "http.StatusForbidden, w.Code", 1)
        text = text.replace('"realtime_sessions_unsupported", payload.Error.Code', '"realtime_sessions_disabled", payload.Error.Code', 1)
        text = text.replace('require.Contains(t, payload.Error.Message, "Gemini Live")', 'require.Contains(t, payload.Error.Message, "disabled")', 1)
        path.write_text(text)
    elif pr == 442:
        # These synthetic contexts do not install tracing middleware or a trace DB.
        # Match the pre-existing real-HTTP diagnostic fixture's explicit no-tracing setup.
        path = root / "relay/adaptor/request_url_security_review_test.go"
        text = path.read_text().replace(
            '"github.com/Laisky/one-api/common/client"',
            '"github.com/Laisky/one-api/common/client"\n "github.com/Laisky/one-api/common/config"',
        )
        text = text.replace(
            "func TestReviewEndpointCredentialDispatch(t *testing.T) {",
            "func TestReviewEndpointCredentialDispatch(t *testing.T) {\n"
            " oldSinks := config.TraceSinks\n"
            " config.TraceSinks = []string{config.TraceSinkNone}\n"
            " t.Cleanup(func() { config.TraceSinks = oldSinks })",
        )
        path.write_text(text)
