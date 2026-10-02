"""One-time, pinned red/green validation and fast-forward-only PR revision."""
from __future__ import annotations
import base64
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import zlib

MAIN = "d7fa5f35ebf3dd6072eace68bd0178196baa0823"
PR = int(sys.argv[1])
BRANCHES = {
    441: "codex/propose-fix-for-quota-bypass-vulnerability",
    442: "codex/propose-fix-for-access-token-leak",
    443: "codex/propose-fix-for-github-models-quota-bypass",
}
HEADS = {
    441: os.environ["REVIEW_HEAD"],
    442: "dadb0d89bf6f6999c3f3df6fbf428becc1a5306b",
    443: "01c7d3177a47d057ae759fb6f24a0603e268ff1a",
}
PACKAGES = {
    441: ["./controller", "./router"],
    442: ["./model", "./relay/billing", "./relay/adaptor", "./relay/controller"],
    443: ["./relay/controller", "./relay/adaptor/openai"],
}
REQUIRED_RED = {
    441: {"TestReviewRealtimeRetiredHandlerNeverMintsCredentials"},
    442: {"TestReviewLogEndpointFailsClosed", "TestReviewHistoricalLogSerialization", "TestReviewBillingSanitizesBothEndpointSources", "TestReviewEndpointCredentialDispatch", "TestReviewResponseDiagnosticsRedactUpstreamURL"},
    443: {"TestReviewProxyRejectsPaidChannels"},
}
ROOT = Path.cwd()
TEMP = Path(os.environ["RUNNER_TEMP"])
OUT = TEMP / f"security-review-{PR}"
OUT.mkdir(exist_ok=True)

def run(args: list[str], cwd: Path = ROOT, check: bool = True) -> subprocess.CompletedProcess:
    """Run non-secret commands and preserve their output for review."""
    print("+", " ".join(args), flush=True)
    return subprocess.run(args, cwd=cwd, text=True, check=check)

def git(*args: str, cwd: Path = ROOT) -> str:
    """Read exact Git state without shell interpolation."""
    return subprocess.check_output(["git", *args], cwd=cwd, text=True)

def test(cwd: Path, phase: str, selector: str, race: bool) -> dict:
    """Require real test assertions, never treating build failures as reproduction."""
    cmd = ["go", "test", "-json", "-count=" + ("3" if race else "1"), "-timeout=10m"]
    if race:
        cmd.append("-race")
    cmd += ["-run", selector, *PACKAGES[PR]]
    print("+", " ".join(cmd), flush=True)
    log = OUT / f"{phase}.jsonl"
    with log.open("w") as output:
        proc = subprocess.Popen(cmd, cwd=cwd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        assert proc.stdout is not None
        for line in proc.stdout:
            output.write(line)
            print(line, end="", flush=True)
        code = proc.wait()
    text = log.read_text()
    events = []
    for line in text.splitlines():
        try:
            events.append(json.loads(line))
        except json.JSONDecodeError:
            pass
    failed = {e["Test"] for e in events if e.get("Action") == "fail" and "Test" in e}
    passed = {e["Test"] for e in events if e.get("Action") == "pass" and "Test" in e}
    result = {"command": cmd, "exit_code": code, "failed_tests": sorted(failed), "passed_tests": sorted(passed)}
    (OUT / f"{phase}-summary.json").write_text(json.dumps(result, indent=2))
    if phase == "red":
        assert code != 0 and REQUIRED_RED[PR] <= failed, result
        assert "[build failed]" not in text and "DATA RACE" not in text and "panic:" not in text, "invalid red control"
    else:
        assert code == 0 and REQUIRED_RED[PR] <= passed and not failed, result
    return result

assert PR in BRANCHES
payload = (ROOT / ".github/security-review-bundle.b64").read_bytes()
assert hashlib.sha256(payload).hexdigest() == "3c43b494c9c2454b3d2fc4366e115bbcbfd5621335f601b873e3597fd373aa5d"
bundle = json.loads(zlib.decompress(base64.b64decode(payload)))
patch = OUT / "review.patch"
patch.write_text(bundle[str(PR)])
run(["git", "fetch", "origin", MAIN, HEADS[PR]])
prepared = TEMP / f"prepared-{PR}"
run(["git", "worktree", "add", "--detach", str(prepared), MAIN])
run(["git", "apply", str(patch)], cwd=prepared)
if PR == 441:
    path = prepared / "controller/realtime.go"
    path.write_text(path.read_text().replace('\t"github.com/Laisky/one-api/relay/adaptor/openai"\n', ""))
paths = git("ls-files", "-m", "-o", "--exclude-standard", cwd=prepared).splitlines()
run(["gofmt", "-w", *[p for p in paths if p.endswith(".go")]], cwd=prepared)
final_files = {path: (prepared / path).read_bytes() for path in paths}
work = TEMP / f"working-{PR}"
run(["git", "worktree", "add", "--detach", str(work), HEADS[PR]])
run(["git", "config", "user.name", "github-actions[bot]"], cwd=work)
run(["git", "config", "user.email", "41898282+github-actions[bot]@users.noreply.github.com"], cwd=work)
merge = run(["git", "merge", "--no-commit", "--no-ff", MAIN], cwd=work, check=False)
conflicts = set(git("diff", "--name-only", "--diff-filter=U", cwd=work).splitlines())
extra_conflict = "relay/billing/billing_compatibility_test.go"
assert conflicts <= set(paths) | ({extra_conflict} if PR == 442 else set()), conflicts
if extra_conflict in conflicts:
    main_test = git("show", MAIN + ":" + extra_conflict)
    old_test = git("show", HEADS[PR] + ":" + extra_conflict)
    begin = old_test.index("// TestPostConsumeQuotaDetailedSanitizesUpstreamEndpoint")
    end = old_test.index("// TestInputValidation", begin)
    (work / extra_conflict).write_text(main_test + "\n" + old_test[begin:end])
    run(["gofmt", "-w", extra_conflict], cwd=work)
for path, data in final_files.items():
    target = work / path
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_bytes(data)
run(["git", "add", "."], cwd=work)
assert not git("diff", "--name-only", "--diff-filter=U", cwd=work).strip()
run(["git", "diff", "--cached", "--check"], cwd=work)
# Keep every new behavior test while restoring only vulnerable production paths.
production = [p for p in paths if p.endswith(".go") and not p.endswith("_test.go")]
if PR == 441:
    production = ["controller/realtime.go"]
elif PR == 443:
    production = ["relay/controller/proxy.go"]
for path in production:
    prior = subprocess.run(["git", "show", MAIN + ":" + path], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
    if prior.returncode == 0:
        (work / path).write_bytes(prior.stdout)
if PR == 442:
    (work / "model/log_metadata_sanitize.go").write_text(git("show", HEADS[PR] + ":model/log_metadata_sanitize.go"))
red_selector = "^(" + "|".join(sorted(REQUIRED_RED[PR])) + ")$"
red = test(work, "red", red_selector, False)
# Restore the exact staged final tree, including the new tests and safety fixes.
run(["git", "checkout-index", "-a", "-f"], cwd=work)
green = test(work, "green", "^TestReview", True)
run(["go", "vet", "./..."], cwd=work)
run(["git", "diff", "--check"], cwd=work)
assert not git("diff", "--name-only", cwd=work).strip(), "tests changed tracked files"
run(["git", "commit", "-m", f"fix(security): close PR {PR} review gaps with behavioral regressions"], cwd=work)
commit = git("rev-parse", "HEAD", cwd=work).strip()
summary = {"pr": PR, "baseline": MAIN, "previous_head": HEADS[PR], "validated_commit": commit, "red": red, "green": green, "vet": "passed"}
(OUT / "summary.json").write_text(json.dumps(summary, indent=2))
(OUT / "final.diff").write_text(git("diff", MAIN, "HEAD", cwd=work))
# Never overwrite a concurrent update. Only the explicitly selected PR branch is writable here.
current = git("ls-remote", "origin", "refs/heads/" + BRANCHES[PR], cwd=work).split()[0]
assert current == HEADS[PR], ("concurrent branch update", current, HEADS[PR])
auth = base64.b64encode(("x-access-token:" + os.environ["GH_TOKEN"]).encode()).decode()
print("::add-mask::" + auth, flush=True)
subprocess.run(["git", "-c", "http.https://github.com/.extraheader=AUTHORIZATION: basic " + auth,
                "push", "origin", "HEAD:refs/heads/" + BRANCHES[PR]], cwd=work, check=True)
print("VALIDATED_AND_PUSHED", PR, commit, flush=True)
