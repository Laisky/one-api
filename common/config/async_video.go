package config

import "github.com/Laisky/one-api/common/env"

// AsyncVideoWaitSeconds bounds the synchronous compatibility wait. Timeout only
// ends the HTTP wait; it never cancels/refunds/replays an accepted provider job.
var AsyncVideoWaitSeconds = max(1, min(600, env.Int("ASYNC_VIDEO_WAIT_SECONDS", 120)))

// AsyncVideoWorkers bounds active provider HTTP operations per gateway replica.
// Database leases coordinate replicas; each operation has a 30-second timeout.
var AsyncVideoWorkers = max(1, min(32, env.Int("ASYNC_VIDEO_WORKERS", 4)))
