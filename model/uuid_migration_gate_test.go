package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// The finalizer compatibility gate: a UUID-aware writer's output must leave catch-up
// nothing to reconcile and must finalize on the first attempt (UUID-A22).

// withLogConsumeEnabled toggles consumption logging for one test.
// RecordConsumeLog silently returns when the gate is off, which would make a log writer case
// assert against a row that was never written.
// Parameters:
//   - t: test handle used for cleanup registration.
//   - enabled: desired gate state.
//
// Return values: none.
func withLogConsumeEnabled(t *testing.T, enabled bool) {
	t.Helper()
	original := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(enabled)
	t.Cleanup(func() { config.SetLogConsumeEnabled(original) })
}

// TestUUIDAwareWriterOutputPassesFinalizerCompatibilityGate covers UUID-A22: this is the point
// of the acceptance item. Rows are created exclusively through UUID-aware writer paths, each
// mirroring what its production call site does, and the resulting database must contain no work
// for catch-up and must pass finalization on the first attempt. A writer that skipped an owned
// or denormalized UUID would leave a missing owned uuid or a fillable gap, and the finalizer
// would refuse to write the marker.
func TestUUIDAwareWriterOutputPassesFinalizerCompatibilityGate(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, true)
	withLogConsumeEnabled(t, true)
	ctx := context.Background()

	// Owners first: every writer below copies a uuid that already exists.
	fixture := newUUIDWriterFixture(t, db)
	invitee := &User{Username: "gate-invitee", Password: "password-hash"}
	require.NoError(t, invitee.Insert(ctx, fixture.inviter.Id))

	require.NoError(t, db.Create(&Trace{TraceId: "gate-trace", URL: "/v1/chat/completions",
		Method: "POST", Timestamps: `{"request_received":1}`}).Error)

	// controller.AddRedemption stamps UserUUID from the request context before Insert.
	redemption := &Redemption{UserId: fixture.user.Id, UserUUID: &fixture.user.UUID,
		Key: "gate-redemption-key", Name: "gate-gift"}
	require.NoError(t, redemption.Insert())

	// model.(*UserRequestCost).Insert resolves the owner uuid itself.
	cost := NewUserRequestCost(fixture.user.Id, "gate-request-id", 10)
	require.NoError(t, cost.Insert())

	// model.UpsertMCPTools copies the server uuid onto every tool.
	require.NoError(t, UpsertMCPTools(fixture.server.Id, fixture.server.UUID,
		[]*MCPTool{{Name: "gate-tool"}}))

	// model.CreatePasskeyCredential resolves the owner uuid itself.
	require.NoError(t, CreatePasskeyCredential(&PasskeyCredential{UserId: fixture.user.Id,
		CredentialName: "gate-passkey", CredentialID: []byte("gate-credential-id"),
		PublicKey: []byte("gate-public-key")}))

	// The relay adaptors stamp every uuid on the binding before saving it.
	require.NoError(t, SaveAsyncTaskBinding(ctx, &AsyncTaskBinding{
		TaskID: "gate-task", TaskType: "video",
		UserID: fixture.user.Id, UserUUID: &fixture.user.UUID,
		TokenID: fixture.token.Id, TokenUUID: &fixture.token.UUID,
		ChannelID: fixture.channel.Id, ChannelUUID: &fixture.channel.UUID,
		ChannelType: 1,
	}))

	// The relay billing path stamps the consume log through SetLogExternalUUIDs.
	consumeLog := &Log{UserId: fixture.user.Id, ChannelId: fixture.channel.Id,
		TokenName: fixture.token.Name, ModelName: "gpt-4o", Quota: 10, Content: "gate consume log"}
	SetLogExternalUUIDs(consumeLog, fixture.user.UUID, fixture.channel.UUID, fixture.token.UUID)
	RecordConsumeLog(ctx, consumeLog)
	require.Positive(t, consumeLog.Id, "the consume log must be persisted")

	// controller/token.go builds the transaction from the token and the log it just recorded.
	require.NoError(t, CreateTokenTransaction(ctx, &TokenTransaction{
		TransactionID: "gate-txn",
		TokenId:       fixture.token.Id, TokenUUID: &fixture.token.UUID,
		UserId: fixture.user.Id, UserUUID: &fixture.user.UUID,
		LogId: &consumeLog.Id, LogUUID: &consumeLog.UUID,
		Status: TokenTransactionStatusPending, PreQuota: 10,
	}))

	// A UUID-aware writer's output leaves catch-up nothing to reconcile.
	settled := runCatchUp(t, topology)
	require.Zero(t, settled.updated,
		"a UUID-aware writer must not create rows that catch-up still has to fill")
	require.False(t, settled.budgetExhausted)
	requireMarker(t, db, externalUUIDPrimaryMigrationKey, false)

	// And it passes finalization on the first attempt. The finalizer is the whole gate:
	// it promotes the unique indexes and only then runs the global validation that rejects a
	// missing owned uuid, a fillable fk gap, or an fk uuid that disagrees with its live owner.
	// validateExternalUUIDs is deliberately not called on its own here, because its index gate
	// cannot pass until that promotion has run.
	_, err := runFinalizer(t, topology)
	require.NoError(t, err, "the finalizer must accept a database written only by UUID-aware writers")
	requireMarker(t, db, externalUUIDPrimaryMigrationKey, true)

	// Every owned table the writers touched carries a canonical uuid; the finalizer's own
	// promotion also proves the unique index over those values.
	for _, target := range uuidOwnedRegistry() {
		var rows int64
		require.NoError(t, db.Table(target.table).Count(&rows).Error)
		require.Positivef(t, rows, "the gate fixture must exercise %s", target.table)
		requireUUIDUniqueIndex(t, db, target)
	}
}

// seedUUIDWriterLegacyLogs inserts count legacy log rows with no UUID values, cycling their
// user_id over the seeded legacy users so the FK phase has real references to resolve.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle receiving the rows.
//   - count: number of legacy log rows.
//   - userCount: number of seeded legacy users to cycle over.
//
// Return values: none.

// TestSQLitePromotionNeedsNoOperatorFlag: the migration must run and complete automatically
// on a default deployment, and SQLite is the default DSN-less deployment. SQLite has no
// online DDL and no maintenance-window concept that maps onto a single process, so its
// unique-index promotion proceeds under the bounded busy retry and context deadline without
// EXTERNAL_UUID_BACKFILL_ALLOW_BLOCKING_DDL; that flag now gates only the MySQL blocking
// fallback.
func TestSQLitePromotionNeedsNoOperatorFlag(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, true)
	withBlockingDDLAllowed(t, false)
	require.NoError(t, db.Exec("INSERT INTO users (id, username, password) VALUES (1, 'root', 'password-hash')").Error)

	_, err := runFinalizer(t, topology)
	require.NoError(t, err, "sqlite finalization must not depend on the blocking-DDL flag")
	requireMarker(t, db, externalUUIDPrimaryMigrationKey, true)
	requireUUIDUniqueIndex(t, db, uuidOwnedTarget{role: uuidRolePrimary, table: "users", model: &User{}})
}

// TestValidationRejectsNonV7OwnedUUID covers the version-7 half of the validation contract:
// a well-formed but non-v7 legacy identifier is not the time-ordered UUIDv7 this project
// promises, so it blocks finalization for explicit operator remediation.
func TestValidationRejectsNonV7OwnedUUID(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, true)

	// A canonical, parseable UUIDv4 rather than a UUIDv7.
	require.NoError(t, db.Exec(
		"INSERT INTO users (id, uuid, username, password) VALUES (1, '9f8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d', 'root', 'password-hash')").Error)

	_, err := runFinalizer(t, topology)
	require.ErrorContains(t, err, "malformed owned uuid")
	requireMarker(t, db, externalUUIDPrimaryMigrationKey, false)

	require.True(t, isCanonicalHyphenatedUUID("018f0000-0000-7000-8000-000000000001"), "a canonical v7 must pass")
	require.False(t, isCanonicalHyphenatedUUID("9f8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d"), "a v4 must not pass")
	require.False(t, isCanonicalHyphenatedUUID("not-a-uuid"), "garbage must not pass")
}
