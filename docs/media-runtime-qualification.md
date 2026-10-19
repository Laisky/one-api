# Offline media runtime qualification

This suite qualifies image packaging without changing application media behavior.
It uses only synthetic local media and local TLS upstream fixtures. Every Docker
container overrides the application entrypoint, disables external networking,
and mounts only corpus, compiled tests, output, and the public tokenizer cache. Each invocation binds its own disposable dev-disk scratch
directory to /tmp, so executable test fixtures work without altering host mount
policy.

## Run

Supply an immutable local baseline, a local candidate, a pre-populated public
tokenizer cache, and a new evidence directory:

    python3 scripts/qualify_media_runtime.py \
      --baseline ppcelery/one-api@sha256:BASELINE_PLATFORM_DIGEST \
      --candidate oneapi:qualification-candidate \
      --tokenizer-cache /path/to/public-tokenizer-cache \
      --evidence /tmp/oneapi-media-qualification

Use --baseline-only instead of --candidate to establish baseline evidence while
a candidate builds. Later use --reuse with that same evidence directory and
--candidate to run the comparison. Reuse checks baseline identity, fixture hashes,
and compiled test binary hashes. It intentionally reuses those binaries; choose a
fresh evidence directory after test/source changes. With --reuse, --rebuild-tests
explicitly refreshes the Go executables when fixing the qualification tests.

The script never pulls images. It compiles actual Go package test executables
with GOMAXPROCS=2 and GOFLAGS=-p=2, then runs those executables in both images.
An explicitly selected runtime test must pass: codec absence and test skips fail
qualification. Regular go test skips the added image-only tests unless
ONEAPI_MEDIA_RUNTIME=1 is supplied. No external fixture downloads are required.

## Coverage and evidence

The baseline generates WAV, extra/odd RIFF chunks, truncated/inconsistent WAV,
CBR/VBR MP3 with and without Xing, M4A, AAC/ADTS, FLAC, Ogg/Opus, Ogg/Vorbis,
WebM, an MP4 edit list, and a multistream MP4 whose video outlasts its audio.
Negative controls include corrupt, empty, truncated MP3, and valid streaming WebM
with unknown duration. Unicode metadata, extensionless files, and misleading
extensions exercise metadata and content detection.

Both images process the identical fixture bytes. Evidence includes image IDs,
architecture, fixture and test-binary hashes, exact probe output/classification,
metadata, and normalized decoded PCM hashes. ASLR addresses are normalized in
diagnostics; substantive errors remain compared. Input duration continues to
mean FFprobe FORMAT duration, including existing container metadata/estimation
semantics. This is not a migration to decoded-sample billing.

The helper executable exercises every corpus case through GetAudioDuration.
Successful cases also pass an unseekable reader into GetAudioTokens, verifying
materialization and duration-to-token multiplication. A blocked FIFO verifies
real FFprobe cancellation and process reaping.

The controller executable sends actual multipart files through RelayAudioHelper
against a local synthetic TLS upstream. An explicit USD-per-second tariff checks
exact independently calculated decimal quota, user/token/request ledgers, and
pre-dispatch rejection without charge for unprobeable inputs. Actual default-MP3 and buffered Opus/SSE Gemini HTTP responses also exercise real
encoding and all billing ledgers. Existing physical transcription, tariff
arithmetic, and Gemini encoder-credit tests also run.

The Gemini TTS executable invokes actual Plan.encode for all six formats at
speeds 0.25, 1, and 4. It checks codec, channels, sample rate, audible decoded tone,
and physical decoded duration. Opus reports its conventional 48 kHz decoder rate;
normalization to 24 kHz checks duration consistently. Raw PCM has no embedded
metadata and is interpreted using the API's mono PCM16LE/24 kHz contract.
Decoded PCM hashes and semantic reports for all 18 cases must match between
images. Encoded artifacts are retained; random Ogg stream serials are not mistaken
for audio regressions. A separate test
observes the actual executable mapping, cancels the encoder, and asserts no
process survives; observing only a wrapper is insufficient.

Each run creates a separate run-* directory with package logs, artifacts, a
summary, candidate observations, and differences. The state
baseline_passed_candidate_pending is not a compatibility pass. The state passed
means both images and the complete differential comparison passed. A failure
retains available evidence and removes only that invocation's uniquely named
temporary Docker containers.

## Boundaries

The suite tests image utility packaging and current application paths. It does
not certify arbitrary GPU/video features, all accepted multimedia formats,
arm64 from an amd64 run, external providers, or a changed billing policy.
Nonroot image startup, storage ownership, CA/timezone behavior, SQLite restarts,
and themes remain covered by scripts/test_runtime_image.py.


## Native ARM64 qualification

The existing CI workflow runs `native_arm64_images` on the public GitHub-hosted
`ubuntu-24.04-arm` runner when image or qualification paths change. Manual runs
opt in with `native_arm64_images`; unrelated changes may skip it. `CI required`
requires success whenever selected, including rejecting missing change outputs.
It retains `contents: read`, checks out the exact PR head without persisted Git
credentials, and uses the runner's existing native Docker builder. It does not
install QEMU, create privileged builders, publish images, or deploy.

`scripts/qualify_native_arm64.py` builds the complete production Dockerfile on
native hardware, records immutable input image identities, and compares original,
ownership-only, and media-closure runtimes using the same actual application
binary. Acceptance includes native ELF/platform checks, default and custom UID/GID
runtime checks, the duration/codec/TTS/billing/cancellation corpus, retained media
file hashes and loader closure, and the existing bounded local TLS fixture.
The local TLS fixture proves explicit fixture-CA handling, not arbitrary remote
endpoints or every provider's default trust configuration.

The job has a fixed deadline and disk/memory floors, isolated test caches, bounded
runtime containers, and always uploads compact evidence without images or build
caches. Resource exhaustion is a failed qualification, not a pass or a reason to
delete unrelated runner data. ARM64 acceptance requires a completed native run;
AMD64 results alone do not qualify it. This check does not replace normal hosted
CodeQL acceptance when the stacked draft progresses through its approved base.
