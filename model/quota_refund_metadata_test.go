package model

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/common/config"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestQuotaRefundMetadataRestart recovers independently failing databases from only persisted state.
func TestQuotaRefundMetadataRestart(t *testing.T) {
	oldGate := config.RefundCostRecoveryEnabled
	config.RefundCostRecoveryEnabled = true
	t.Cleanup(func() { config.RefundCostRecoveryEnabled = oldGate })
	for _, target := range []string{"logs", "user_request_costs"} {
		for _, newerRetry := range []bool{false, true} {
			t.Run(target+map[bool]string{false: "/original", true: "/newer_equal_charge"}[newerRetry], func(t *testing.T) {
				primaryPath, intent := quotaRefundFixture(t)
				require.NoError(t, DB.AutoMigrate(&UserRequestCost{}))
				logPath := filepath.Join(t.TempDir(), "logs.db")
				logs, err := gorm.Open(sqlite.Open(logPath), &gorm.Config{})
				require.NoError(t, err)
				require.NoError(t, logs.AutoMigrate(&Log{}))
				oldLogs := LOG_DB
				LOG_DB = logs
				t.Cleanup(func() {
					pool, e := LOG_DB.DB()
					require.NoError(t, e)
					require.NoError(t, pool.Close())
					LOG_DB = oldLogs
				})
				entry := Log{UserId: intent.UserID, RequestId: intent.RequestID, Quota: 300, Metadata: LogMetadata{LogMetadataKeyProvisional: true}}
				require.NoError(t, LOG_DB.Create(&entry).Error)
				require.NoError(t, UpdateUserRequestCostQuotaByRequestID(intent.UserID, intent.RequestID, 300))
				require.NoError(t, PrepareQuotaRefundMetadata(context.Background(), &intent, entry.Id))
				require.NoError(t, EnsureQuotaRefund(context.Background(), intent))
				faulty := DB
				if target == "logs" {
					faulty = LOG_DB
				}
				require.NoError(t, faulty.Callback().Update().Before("gorm:update").Register("metadata:outage", func(tx *gorm.DB) {
					if tx.Statement.Table == target {
						tx.AddError(errors.New("synthetic separate database outage"))
					}
				}))
				require.Error(t, ApplyQuotaRefund(context.Background(), intent.ID))
				assertQuotaRefundBalance(t, 10000)
				var pending QuotaRefund
				require.NoError(t, DB.First(&pending, "id = ?", intent.ID).Error)
				require.Equal(t, QuotaRefundCompleted, pending.Status)
				require.False(t, pending.MetadataCompleted)
				for _, db := range []*gorm.DB{DB, LOG_DB} {
					pool, e := db.DB()
					require.NoError(t, e)
					require.NoError(t, pool.Close())
				}
				DB, err = gorm.Open(sqlite.Open(primaryPath), &gorm.Config{})
				require.NoError(t, err)
				LOG_DB, err = gorm.Open(sqlite.Open(logPath), &gorm.Config{})
				require.NoError(t, err)
				if newerRetry {
					// An equal-valued successful retry must still advance generation.
					require.NoError(t, UpdateUserRequestCostQuotaByRequestID(intent.UserID, intent.RequestID, 300))
				}
				require.NoError(t, RecoverQuotaRefunds(context.Background()))
				require.NoError(t, RecoverQuotaRefunds(context.Background()))
				require.NoError(t, ApplyQuotaRefund(context.Background(), intent.ID))
				assertQuotaRefundBalance(t, 10000)
				var recovered Log
				require.NoError(t, LOG_DB.First(&recovered, entry.Id).Error)
				require.Zero(t, recovered.Quota)
				require.NotContains(t, recovered.Metadata, LogMetadataKeyProvisional)
				var cost UserRequestCost
				require.NoError(t, DB.First(&cost, intent.CostRecordID).Error)
				want := int64(0)
				if newerRetry {
					want = 300
				}
				require.Equal(t, want, cost.Quota)
				require.NoError(t, DB.First(&pending, "id = ?", intent.ID).Error)
				require.True(t, pending.MetadataCompleted)
				changed := intent
				changed.CostRevision++
				require.Error(t, EnsureQuotaRefund(context.Background(), changed), "immutable target must not be rebound")
			})
		}
	}
}

// TestRefundCostRevisionCompatibility preserves zero-generation rows and advances every ordinary writer.
func TestRefundCostRevisionCompatibility(t *testing.T) {
	_, intent := quotaRefundFixture(t)
	require.NoError(t, DB.AutoMigrate(&UserRequestCost{}))
	entry := NewUserRequestCost(intent.UserID, intent.RequestID, 300)
	require.NoError(t, DB.Create(entry).Error)
	require.Zero(t, entry.Revision, "legacy/default generation remains valid")
	require.NoError(t, UpdateUserRequestCostQuotaByRequestID(intent.UserID, intent.RequestID, 300))
	var updated UserRequestCost
	require.NoError(t, DB.First(&updated, entry.Id).Error)
	require.EqualValues(t, 1, updated.Revision)
}

// TestRefundMetadataMixedVersionGate prevents later opt-in from trusting a legacy-writer snapshot.
func TestRefundMetadataMixedVersionGate(t *testing.T) {
	_, intent := quotaRefundFixture(t)
	oldGate := config.RefundCostRecoveryEnabled
	config.RefundCostRecoveryEnabled = false
	t.Cleanup(func() { config.RefundCostRecoveryEnabled = oldGate })
	require.NoError(t, DB.AutoMigrate(&UserRequestCost{}))
	require.NoError(t, UpdateUserRequestCostQuotaByRequestID(intent.UserID, intent.RequestID, 300))
	require.NoError(t, PrepareQuotaRefundMetadata(context.Background(), &intent, 0))
	require.False(t, intent.CostRecoveryEligible)
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register("metadata:cost_outage", func(tx *gorm.DB) {
		if tx.Statement.Table == "user_request_costs" {
			tx.AddError(errors.New("synthetic cost outage"))
		}
	}))
	persisted, err := RefundCurrentAttemptWithRecovery(context.Background(), intent)
	require.True(t, persisted)
	require.Error(t, err)
	require.NoError(t, DB.Callback().Update().Remove("metadata:cost_outage"))
	assertQuotaRefundBalance(t, 10000)
	require.NoError(t, RecoverQuotaRefunds(context.Background()), "manual-only work must not occupy recovery batches")
	// An old binary writes equal cost without advancing generation.
	require.NoError(t, DB.Model(&UserRequestCost{}).Where("id = ?", intent.CostRecordID).Update("quota", 300).Error)
	config.RefundCostRecoveryEnabled = true
	require.NoError(t, DB.Model(&QuotaRefund{}).Where("id = ?", intent.ID).Update("retry_at", 0).Error)
	require.NoError(t, RecoverQuotaRefunds(context.Background()), "later opt-in cannot qualify old snapshot")
	require.Error(t, ApplyQuotaRefund(context.Background(), intent.ID))
	persisted, err = RefundQuotaWithRecovery(context.Background(), intent)
	require.True(t, persisted)
	require.Error(t, err, "public replay helper has no original-attempt authority")
	persisted, err = RefundCurrentAttemptWithRecovery(context.Background(), intent)
	require.True(t, persisted)
	require.Error(t, err, "re-entering original wrapper cannot reclaim authority for persisted intent")
	var cost UserRequestCost
	require.NoError(t, DB.First(&cost, intent.CostRecordID).Error)
	require.EqualValues(t, 300, cost.Quota)
	assertQuotaRefundBalance(t, 10000)
	var row QuotaRefund
	require.NoError(t, DB.First(&row, "id = ?", intent.ID).Error)
	require.False(t, row.MetadataCompleted)
}

// TestRefundMetadataSchemaUpgrade preserves historical rows without enabling recovery for them.
func TestRefundMetadataSchemaUpgrade(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "legacy.db")), &gorm.Config{})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	require.NoError(t, db.Exec("CREATE TABLE quota_refunds (id varchar(36) PRIMARY KEY, token_id integer, user_id integer, amount bigint, request_id text, reason text, status text, retry_at bigint, attempts bigint, completed_at bigint, created_at bigint)").Error)
	require.NoError(t, db.Exec("INSERT INTO quota_refunds (id,token_id,user_id,amount,request_id,reason,status,retry_at,attempts,completed_at,created_at) VALUES ('old',1,2,300,'old-request','legacy','completed',0,0,1,1)").Error)
	require.NoError(t, db.Exec("CREATE TABLE user_request_costs (id integer PRIMARY KEY, user_id integer, request_id text, quota bigint)").Error)
	require.NoError(t, db.Exec("INSERT INTO user_request_costs (id,user_id,request_id,quota) VALUES (1,2,'old-request',300)").Error)
	require.NoError(t, db.AutoMigrate(&QuotaRefund{}, &UserRequestCost{}))
	require.True(t, db.Migrator().HasIndex(&QuotaRefund{}, "idx_quota_refund_pending"))
	require.True(t, db.Migrator().HasIndex(&QuotaRefund{}, "idx_quota_refund_metadata"))
	var refund QuotaRefund
	require.NoError(t, db.First(&refund, "id = ?", "old").Error)
	require.Equal(t, QuotaRefundCompleted, refund.Status)
	require.EqualValues(t, 300, refund.Amount)
	require.False(t, refund.MetadataRequested)
	require.False(t, refund.CostRecoveryEligible)
	var cost UserRequestCost
	require.NoError(t, db.First(&cost, 1).Error)
	require.EqualValues(t, 300, cost.Quota)
	require.Zero(t, cost.Revision)
}

// TestRefundAsyncCostWriterRevision ensures async receipt replay cannot evade shared cost fencing.
func TestRefundAsyncCostWriterRevision(t *testing.T) {
	_, task := durableJobTestStore(t)
	task = reserveDurableJob(t, task)
	require.NoError(t, writeAsyncTaskRequestCost(context.Background(), task))
	var before UserRequestCost
	require.NoError(t, DB.Where("request_id = ?", task.RequestID).Take(&before).Error)
	require.NoError(t, writeAsyncTaskRequestCost(context.Background(), task))
	var after UserRequestCost
	require.NoError(t, DB.First(&after, before.Id).Error)
	require.Equal(t, before.Quota, after.Quota)
	require.Equal(t, before.Revision+1, after.Revision)
}

// TestRefundRecoveryPrioritizesCredit prevents manual metadata work from starving financial recovery.
func TestRefundRecoveryPrioritizesCredit(t *testing.T) {
	_, pending := quotaRefundFixture(t)
	oldGate := config.RefundCostRecoveryEnabled
	config.RefundCostRecoveryEnabled = false
	t.Cleanup(func() { config.RefundCostRecoveryEnabled = oldGate })
	for i := range 40 {
		row := QuotaRefund{ID: fmt.Sprintf("manual-%d", i), TokenID: 102, UserID: 101, Amount: 1, Status: QuotaRefundCompleted, MetadataRequested: true, LogMetadataCompleted: true, CostSnapshotUnavailable: true, RetryAt: -1}
		require.NoError(t, DB.Create(&row).Error)
	}
	require.NoError(t, EnsureQuotaRefund(context.Background(), pending))
	require.NoError(t, RecoverQuotaRefunds(context.Background()))
	assertQuotaRefundBalance(t, 10000)
	var manual QuotaRefund
	require.NoError(t, DB.First(&manual, "id = ?", "manual-0").Error)
	require.Zero(t, manual.Attempts, "manual-only work is not periodically rewritten")
}
