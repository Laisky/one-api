package model

// Fixtures for the compact UUID fault-injection suite (AUTO-T09/T10/T11/T12).
//
// This file carries only the shared apparatus — the live fixture, the concurrent workload, the
// database relay, and the marker/digest assertions. The tests themselves, and the reasoning
// about what a "kill" means, live in compact_uuid_fault_test.go. The split exists because the
// suite exceeds the repository's 600-line file limit; every symbol here is prefixed compactFault
// so it can never collide with the rest of the package.

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
)

const (
	// compactFaultHooksEnv optionally names in-process fault hooks. This suite reads it only to
	// report which set was active: every fault it injects comes from outside the coordinator
	// (context, pool, relay, lock) rather than through a seam compiled into production code.
	compactFaultHooksEnv = "COMPACT_UUID_TEST_FAULT_HOOKS"
	// compactFaultSeedRows is the pre-expansion fixture size for tests that must converge. It is
	// deliberately small: see compactFaultPartialBackfillBarrier for the defect that stops any
	// larger users fixture from ever completing.
	compactFaultSeedRows = 200
	// compactFaultBacklogRows is the fixture size for the partial-backfill barrier, which needs
	// a fill that provably cannot finish in one cycle.
	compactFaultBacklogRows = 2500
	// compactFaultTrafficBase is the first row id the workload owns, far above the fixture.
	compactFaultTrafficBase = 1_000_000
	// compactFaultTrafficStride separates each worker's private id range.
	compactFaultTrafficStride = 100_000
	// compactFaultUpdateBase is the first index of the private UUID space rewrites draw from.
	compactFaultUpdateBase = 30_000_000
	// compactFaultUpdateStride separates each worker's private rewrite UUID space.
	compactFaultUpdateStride = 1_000_000
	// compactFaultAbsentOffset derives a UUID that is never written, for the not-found probe.
	compactFaultAbsentOffset = 20_000_000
	// compactFaultCreateEvery is how often an iteration creates a row rather than rewriting one.
	//
	// The workload is deliberately create-light and read/update-heavy. That is closer to the
	// proposal's own mix (30% creates/updates, 40% exact reads) than an insert-per-iteration
	// loop, and it keeps the table's row count bounded so a long hold measures the contract
	// rather than the size of a table the test itself inflated.
	compactFaultCreateEvery = 16
	// compactFaultPace paces one worker, keeping the load a request stream rather than a tight
	// loop that would measure the driver instead of the contract.
	compactFaultPace = 20 * time.Millisecond
	// compactFaultHoldFor is how long each barrier is held under traffic.
	compactFaultHoldFor = 3 * time.Second
	// compactFaultForegroundBound is the proposal's foreground blocking ceiling.
	compactFaultForegroundBound = 5 * time.Second
	// compactFaultMaxCycles bounds every cycle loop so a stall fails loudly.
	compactFaultMaxCycles = 200
	// compactFaultOldSettle is how long a pinned artifact runs before it is stopped.
	compactFaultOldSettle = 16 * time.Second
	// compactFaultShadowCountSQL counts every compact shadow column that exists.
	compactFaultShadowCountSQL = "SELECT count(*) FROM information_schema.columns " +
		"WHERE table_schema = 'public' AND column_name LIKE '%_compact'"
)

// compactFaultCancelDelays sweeps where a kill lands inside a cycle: at the first statement, and
// at a spread of depths reaching into DDL, batch, validation, and marker work. The values are
// fixed rather than random so a failure reproduces exactly.
var compactFaultCancelDelays = []time.Duration{
	0, time.Millisecond, 2 * time.Millisecond, 5 * time.Millisecond,
	10 * time.Millisecond, 25 * time.Millisecond, 50 * time.Millisecond, 100 * time.Millisecond,
}

// compactFaultDialect returns the PostgreSQL descriptor this suite qualifies.
// Parameters: none.
//
// Return values:
//   - compactLiveDialect: the PostgreSQL engine descriptor.
func compactFaultDialect() compactLiveDialect { return compactLiveDialects()[1] }

// compactFaultFixture builds a clean live topology, optionally bounds the row budget, and seeds it.
// Parameters:
//   - t: test handle used for assertions and cleanup.
//   - rows: fixture size.
//   - budget: per-cycle row budget to hold for this test, or zero to keep the shipped default.
//
// Return values:
//   - *gorm.DB: primary handle.
//   - *databaseTopology: unified topology.
//   - context.Context: seeded context bounding the whole test.
func compactFaultFixture(t *testing.T, rows int, budget int) (*gorm.DB, *databaseTopology, context.Context) {
	t.Helper()
	dialect := compactFaultDialect()
	db, topology, ok := newLiveCompactTopology(t, dialect, false)
	if !ok {
		compactLiveSkipf(t, "%s is not configured", dialect.primaryEnv)
	}
	if budget > 0 {
		original := config.CompactUUIDMaxRowsPerCycle
		config.CompactUUIDMaxRowsPerCycle = budget
		t.Cleanup(func() { config.CompactUUIDMaxRowsPerCycle = original })
	}

	ctx, cancel := context.WithTimeout(withCompactLogger(context.Background()), 15*time.Minute)
	t.Cleanup(cancel)
	compactFaultSeed(t, db, rows)
	return db, topology, ctx
}

// compactFaultSeed writes the pre-expansion fixture as any legacy writer would: text only.
// Parameters:
//   - t: test handle used for assertions.
//   - db: live handle.
//   - rows: number of rows to seed.
//
// Return values: none.
func compactFaultSeed(t *testing.T, db *gorm.DB, rows int) {
	t.Helper()
	require.NoError(t, db.Exec(
		"INSERT INTO users (id, username, password, uuid) "+
			"SELECT g, 'cf-seed-' || g, 'x', '018f0000-0000-7000-8000-' || lpad(to_hex(g), 12, '0') "+
			"FROM generate_series(1, ?) AS g", rows).Error)
	require.Equal(t, compactUUIDTextFor(rows), compactFaultText(db, "SELECT uuid FROM users WHERE id = ?", rows),
		"the server-side fixture must produce exactly this suite's own UUID vectors")
}

// compactFaultText reads one trimmed string scalar, or an empty string for NULL or an error.
//
// Trimming is required and is not a weakened assertion: the legacy column is CHAR(36), so
// PostgreSQL's bpchar pads a shorter value on the wire while defining equality and length to
// ignore that padding. The live suite trims for the same reason.
// Parameters:
//   - db: live handle.
//   - query: statement returning one column.
//   - args: bind values.
//
// Return values:
//   - string: the trimmed scalar.
func compactFaultText(db *gorm.DB, query string, args ...any) string {
	var value *string
	if err := db.Raw(query, args...).Scan(&value).Error; err != nil || value == nil {
		return ""
	}
	return strings.TrimRight(*value, " ")
}

// compactFaultCount reads one integer scalar.
// Parameters:
//   - t: test handle used for assertions.
//   - db: live handle.
//   - query: statement returning one count.
//
// Return values:
//   - int: the count.
func compactFaultCount(t *testing.T, db *gorm.DB, query string) int {
	t.Helper()
	var count int
	require.NoError(t, db.Raw(query).Scan(&count).Error)
	return count
}

// compactFaultDigest fingerprints the committed authoritative bytes of the fixture.
//
// It covers text only. Compact shadows legitimately change as the migration proceeds; the
// authoritative text is what must never move, and that is what "committed data is stable" means.
// Parameters:
//   - db: live handle.
//   - maxID: highest fixture id to include, excluding the traffic range.
//
// Return values:
//   - string: digest of every fixture row's id and text.
func compactFaultDigest(db *gorm.DB, maxID int) string {
	return compactFaultText(db,
		"SELECT md5(string_agg(id || ':' || rtrim(uuid), ',' ORDER BY id)) FROM users WHERE id <= ?", maxID)
}

// compactFaultMarkerIntegrity reports whether a marker exists, asserting that any that does is true.
//
// This is the "no false marker" assertion, and it is deliberately the only way this suite asks
// about markers, so no call site can check for presence and forget to check for truth. A marker
// claims the historical installation completed and validated, so one that exists after an
// arbitrary kill must have objects that verify against the live catalog and global equality
// fingerprints that match. A missing marker is always acceptable: the application serves
// authoritative text either way.
// Parameters:
//   - t: test handle used for assertions.
//   - ctx: context bounding the reads and audits.
//   - topology: topology under test.
//
// Return values:
//   - bool: true when a marker is present and was proven genuine.
func compactFaultMarkerIntegrity(t *testing.T, ctx context.Context, topology *databaseTopology) bool {
	t.Helper()
	markers, err := readCompactMarkerState(ctx, topology)
	require.NoError(t, err)
	if !markers.anyPresent() {
		return false
	}
	verified, reason, err := validateCompactObjects(ctx, topology)
	require.NoError(t, err)
	require.True(t, verified, "a written marker must never outrun its objects: %s", reason)
	_, matched, err := verifyCompactFingerprints(ctx, topology)
	require.NoError(t, err)
	require.True(t, matched, "a written marker must never outrun global equality")
	return true
}

// compactFaultTraffic is the concurrent legacy workload held against every barrier.
type compactFaultTraffic struct {
	// stop closes to retire every worker.
	stop chan struct{}
	// wg joins the workers.
	wg sync.WaitGroup
	// ops counts completed operations.
	ops atomic.Int64
	// maxLatency records the slowest single operation, in nanoseconds.
	maxLatency atomic.Int64
	// mu guards acked and failures.
	mu sync.Mutex
	// acked maps every acknowledged row id to the text its last acknowledged write stored.
	acked map[int]string
	// failures collects every unexpected error, so a test can name the first one.
	failures []error
}

// compactFaultStartTraffic starts the concurrent workload against the live users table.
//
// Every worker owns a private id range and private UUID vectors, so a lost or duplicated row is
// unambiguously the database's doing rather than two workers colliding.
// Parameters:
//   - t: test handle used for cleanup registration.
//   - db: live handle.
//   - workers: concurrent client count.
//
// Return values:
//   - *compactFaultTraffic: the running workload.
func compactFaultStartTraffic(t *testing.T, db *gorm.DB, workers int) *compactFaultTraffic {
	t.Helper()
	traffic := &compactFaultTraffic{stop: make(chan struct{}), acked: map[int]string{}}
	for worker := 0; worker < workers; worker++ {
		traffic.wg.Add(1)
		go traffic.run(db, worker)
	}
	t.Cleanup(traffic.halt)
	return traffic
}

// halt retires the workload without asserting anything.
// Parameters: none.
//
// Return values: none.
func (traffic *compactFaultTraffic) halt() {
	select {
	case <-traffic.stop:
	default:
		close(traffic.stop)
	}
	traffic.wg.Wait()
}

// do times one operation, records it, and reports whether the worker may continue.
// Parameters:
//   - fn: the operation.
//
// Return values:
//   - bool: false when the operation failed and the worker must retire.
func (traffic *compactFaultTraffic) do(fn func() error) bool {
	started := time.Now()
	err := fn()
	elapsed := time.Since(started).Nanoseconds()
	for {
		previous := traffic.maxLatency.Load()
		if elapsed <= previous || traffic.maxLatency.CompareAndSwap(previous, elapsed) {
			break
		}
	}
	if err != nil {
		traffic.mu.Lock()
		traffic.failures = append(traffic.failures, err)
		traffic.mu.Unlock()
		return false
	}
	traffic.ops.Add(1)
	return true
}

// ack records one acknowledged write.
// Parameters:
//   - id: row id.
//   - text: the exact authoritative text the write stored.
//
// Return values: none.
func (traffic *compactFaultTraffic) ack(id int, text string) {
	traffic.mu.Lock()
	traffic.acked[id] = text
	traffic.mu.Unlock()
}

// firstFailure returns the first unexpected error observed so far, or nil.
// Parameters: none.
//
// Return values:
//   - error: the first failure, or nil.
func (traffic *compactFaultTraffic) firstFailure() error {
	traffic.mu.Lock()
	defer traffic.mu.Unlock()
	if len(traffic.failures) == 0 {
		return nil
	}
	return traffic.failures[0]
}

// run is one client: create or rewrite a row, read it back exactly, resolve it, then pace.
//
// Row ids and UUID vectors are drawn from per-worker private spaces, and a rewrite draws a
// fresh vector rather than reusing one, so every acknowledged write is globally unique and a
// duplicate or a stale read is unambiguous.
//
// The resolution probe goes through the production entry point, so whichever path this
// process's health gate selects — the verified compact index or the legacy text index — must
// produce the same correct answer, and a UUID that was never written must produce a correct
// not-found rather than somebody else's row.
// Parameters:
//   - db: live handle.
//   - worker: worker index, deciding its private id and UUID ranges.
//
// Return values: none.
func (traffic *compactFaultTraffic) run(db *gorm.DB, worker int) {
	defer traffic.wg.Done()
	ctx := withCompactLogger(context.Background())
	target, err := compactLookupTarget("users")
	if err != nil {
		traffic.mu.Lock()
		traffic.failures = append(traffic.failures, errors.Wrap(err, "resolve users lookup target"))
		traffic.mu.Unlock()
		return
	}

	owned, rewrites := []int{}, 0
	for iteration := 0; ; iteration++ {
		select {
		case <-traffic.stop:
			return
		default:
		}

		var id int
		var text string
		if len(owned) == 0 || iteration%compactFaultCreateEvery == 0 {
			id = compactFaultTrafficBase + worker*compactFaultTrafficStride + len(owned)
			text = compactUUIDTextFor(id)
			if !traffic.do(func() error {
				return db.Exec("INSERT INTO users (id, username, password, uuid) VALUES (?, ?, 'x', ?)",
					id, fmt.Sprintf("cf-%d", id), text).Error
			}) {
				return
			}
			owned = append(owned, id)
		} else {
			rewrites++
			id = owned[iteration%len(owned)]
			text = compactUUIDTextFor(compactFaultUpdateBase + worker*compactFaultUpdateStride + rewrites)
			if !traffic.do(func() error {
				return db.Exec("UPDATE users SET uuid = ? WHERE id = ?", text, id).Error
			}) {
				return
			}
		}
		traffic.ack(id, text)

		if !traffic.do(func() error {
			if stored := compactFaultText(db, "SELECT uuid FROM users WHERE id = ?", id); stored != text {
				return errors.Errorf("row %d read back %q after acknowledging %q", id, stored, text)
			}
			return nil
		}) {
			return
		}
		if !traffic.do(func() error { return compactFaultResolve(ctx, db, target, id, text) }) {
			return
		}
		time.Sleep(compactFaultPace)
	}
}

// compactFaultResolve asserts one exact resolution and one correct not-found.
// Parameters:
//   - ctx: context bounding the lookups.
//   - db: live handle.
//   - target: the users lookup target.
//   - id: the row that must be resolved.
//   - text: that row's current authoritative text.
//
// Return values:
//   - error: wrapped error describing a stale, wrong, or unexpected resolution.
func compactFaultResolve(ctx context.Context, db *gorm.DB, target compactTarget, id int, text string) error {
	resolved, err := resolveIDByUUID(ctx, db, target, text)
	if err != nil {
		return errors.Wrapf(err, "resolve acknowledged row %d", id)
	}
	if resolved != int64(id) {
		return errors.Errorf("stale resolution: row %d's current uuid resolved to %d", id, resolved)
	}
	if _, err = resolveIDByUUID(ctx, db, target,
		compactUUIDTextFor(id+compactFaultAbsentOffset)); !errors.Is(err, idresolveErrNotFoundForTest()) {
		return errors.Errorf("an absent uuid must resolve to not-found, got %v", err)
	}
	return nil
}

// stopAndReconcile retires the workload and reconciles every acknowledged write exactly.
//
// The reconciliation runs in both directions: every acknowledged id must exist exactly once
// with exactly the acknowledged text (nothing lost), and no id in the traffic range may exist
// that was never acknowledged (nothing duplicated or invented).
// Parameters:
//   - t: test handle used for assertions.
//   - db: live handle.
//
// Return values: none.
func (traffic *compactFaultTraffic) stopAndReconcile(t *testing.T, db *gorm.DB) {
	t.Helper()
	traffic.halt()
	require.NoError(t, traffic.firstFailure(), "the workload must observe no unexpected error")

	rows := []struct {
		ID   int    `gorm:"column:id"`
		UUID string `gorm:"column:uuid"`
	}{}
	require.NoError(t, db.Raw("SELECT id, rtrim(uuid) AS uuid FROM users WHERE id >= ? ORDER BY id",
		compactFaultTrafficBase).Scan(&rows).Error)

	traffic.mu.Lock()
	defer traffic.mu.Unlock()
	require.Len(t, rows, len(traffic.acked), "every acknowledged row must exist exactly once")
	seen := map[int]bool{}
	for _, row := range rows {
		require.False(t, seen[row.ID], "row %d was returned twice", row.ID)
		seen[row.ID] = true
		expected, acked := traffic.acked[row.ID]
		require.True(t, acked, "row %d exists but was never acknowledged", row.ID)
		require.Equal(t, expected, row.UUID, "row %d lost its acknowledged text", row.ID)
	}
	for id := range traffic.acked {
		require.True(t, seen[id], "acknowledged row %d was lost", id)
	}
	require.Less(t, time.Duration(traffic.maxLatency.Load()), compactFaultForegroundBound,
		"no foreground operation may block for five seconds")
	t.Logf("workload completed %d operations; slowest single operation %s",
		traffic.ops.Load(), time.Duration(traffic.maxLatency.Load()))
}

// compactFaultProxy is a TCP relay that can drop the database out from under a live pool.
//
// A relay rather than a closed pool or a dead port is what makes the recovery half of AUTO-T12
// honest: the SAME gorm handle, with the same pool, must survive the outage and resume on its
// own once the fault is removed. A test that opened a fresh handle after the outage would prove
// only that gorm.Open works.
type compactFaultProxy struct {
	// listener accepts the client side.
	listener net.Listener
	// target is the real server's address.
	target string
	// mu guards open and conns.
	mu sync.Mutex
	// open reports whether new connections are relayed.
	open bool
	// conns tracks every live socket so an outage can sever all of them at once.
	conns map[net.Conn]struct{}
}

// compactFaultStartProxy starts a relay to a real server address.
// Parameters:
//   - t: test handle used for assertions and cleanup.
//   - target: the real server's host:port.
//
// Return values:
//   - *compactFaultProxy: the running relay.
func compactFaultStartProxy(t *testing.T, target string) *compactFaultProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	proxy := &compactFaultProxy{listener: listener, target: target, open: true, conns: map[net.Conn]struct{}{}}
	go func() {
		for {
			client, err := proxy.listener.Accept()
			if err != nil {
				return
			}
			proxy.mu.Lock()
			open := proxy.open
			proxy.mu.Unlock()
			if !open {
				// An outage refuses new connections as well as severing live ones; otherwise
				// the pool would simply reconnect around the fault.
				_ = client.Close()
				continue
			}
			go proxy.relay(client)
		}
	}()
	t.Cleanup(func() { _ = listener.Close(); proxy.setOpen(false) })
	return proxy
}

// relay pipes one client connection to the real server in both directions.
// Parameters:
//   - client: the accepted connection.
//
// Return values: none.
func (proxy *compactFaultProxy) relay(client net.Conn) {
	server, err := net.DialTimeout("tcp", proxy.target, 5*time.Second)
	if err != nil {
		_ = client.Close()
		return
	}
	proxy.mu.Lock()
	proxy.conns[client] = struct{}{}
	proxy.conns[server] = struct{}{}
	proxy.mu.Unlock()
	go func() { _, _ = io.Copy(server, client); _ = server.Close() }()
	go func() { _, _ = io.Copy(client, server); _ = client.Close() }()
}

// setOpen injects or removes the outage.
// Parameters:
//   - open: false severs every live socket and refuses new ones; true restores the relay.
//
// Return values: none.
func (proxy *compactFaultProxy) setOpen(open bool) {
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	proxy.open = open
	if open {
		return
	}
	for conn := range proxy.conns {
		_ = conn.Close()
	}
	proxy.conns = map[net.Conn]struct{}{}
}

// compactFaultRedirectDSN rewrites a live DSN so its host:port points at the
// relay address, returning the rewritten DSN and the original host:port target.
// Both URL DSNs (postgres://user:pass@host:port/db?params) and libpq keyword
// DSNs (host=... port=...) are accepted, mirroring what pr.yml and the manual
// qualification runs set in COMPACT_UUID_TEST_POSTGRES_DSN.
// Parameters:
//   - dsn: the live DSN to rewrite.
//   - addr: relay listener address in host:port form, or "" to keep the DSN as-is.
//
// Return values:
//   - string: the DSN with host:port replaced by addr when addr is non-empty.
//   - string: the original host:port for the relay to dial.
//   - error: wrapped parse failures, or an error when the DSN names no host/port.
func compactFaultRedirectDSN(dsn string, addr string) (string, string, error) {
	if strings.Contains(dsn, "://") {
		parsed, err := url.Parse(dsn)
		if err != nil {
			return "", "", errors.Wrap(err, "parse DSN url")
		}
		if parsed.Hostname() == "" || parsed.Port() == "" {
			return "", "", errors.New("dsn must name a host and a port")
		}
		target := net.JoinHostPort(parsed.Hostname(), parsed.Port())
		if addr != "" {
			parsed.Host = addr
		}
		return parsed.String(), target, nil
	}

	host, port := "", ""
	if addr != "" {
		split, splitPort, err := net.SplitHostPort(addr)
		if err != nil {
			return "", "", errors.Wrap(err, "split relay address")
		}
		host, port = split, splitPort
	}
	fields, server := []string{}, map[string]string{}
	for _, part := range strings.Fields(dsn) {
		key, value, _ := strings.Cut(part, "=")
		server[key] = value
		switch {
		case key == "host" && host != "":
			part = "host=" + host
		case key == "port" && port != "":
			part = "port=" + port
		}
		fields = append(fields, part)
	}
	if server["host"] == "" || server["port"] == "" {
		return "", "", errors.New("dsn must name a host and a port")
	}
	return strings.Join(fields, " "), net.JoinHostPort(server["host"], server["port"]), nil
}
