package model

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/zap"
)

const (
	AsyncTaskReserved       = "reserved"
	AsyncTaskSubmitting     = "submitting"
	AsyncTaskUnknown        = "submission_unknown"
	AsyncTaskQueued         = "queued"
	AsyncTaskRunning        = "running"
	AsyncTaskCompleted      = "completed"
	AsyncTaskFailed         = "failed"
	AsyncTaskCancelled      = "cancelled"
	AsyncTaskReconciliation = "reconciliation_required"
	AsyncBillingHeld        = "held"
	AsyncBillingSettled     = "settled"
	AsyncBillingRefunded    = "refunded"
	MaxAsyncTaskBody        = 1 << 20
)

var ErrAsyncIdempotencyConflict = errors.New("idempotency key was already used for a different request")
var ErrAsyncQuota = errors.New("insufficient quota for asynchronous task")
var ErrAsyncLeaseLost = errors.New("asynchronous task lease is no longer owned")

// AsyncTask is a durable job and quota reservation, independent of an HTTP
// connection. Credentials are never stored here. The private input is erased
// once submission is acknowledged; results and idempotency receipts remain for
// 30 days after financially settled terminal work. Unknown outcomes are retained.
type AsyncTask struct {
	ID              string `gorm:"primaryKey;size:64" json:"-"`
	DedupKey        string `gorm:"size:64;uniqueIndex;not null" json:"-"`
	RequestHash     string `gorm:"size:64;not null" json:"-"`
	UserID          int    `gorm:"index;not null" json:"-"`
	UserUUID        string `gorm:"size:36" json:"-"`
	TokenID         int    `json:"-"`
	TokenUUID       string `gorm:"size:36" json:"-"`
	TokenUnlimited  bool   `json:"-"`
	TokenName       string `gorm:"size:191" json:"-"`
	ChannelID       int    `json:"-"`
	ChannelUUID     string `gorm:"size:36" json:"-"`
	ChannelType     int    `json:"-"`
	BaseURL         string `gorm:"size:2048" json:"-"`
	OriginModel     string `gorm:"size:128" json:"-"`
	ActualModel     string `gorm:"size:128" json:"-"`
	RequestBody     string `gorm:"size:1048576" json:"-"`
	RequestID       string `gorm:"size:128" json:"-"`
	TraceID         string `gorm:"size:128" json:"-"`
	UpstreamID      string `gorm:"size:191" json:"-"`
	State           string `gorm:"size:32;index:idx_async_jobs_due,priority:1" json:"-"`
	BillingState    string `gorm:"size:16;index" json:"-"`
	Quota           int64  `json:"-"`
	QuotedQuota     int64  `gorm:"not null;default:0" json:"-"`
	CostQuotaPerUSD string `gorm:"size:128" json:"-"`
	UpstreamCostUSD string `gorm:"size:128" json:"-"`
	// Observed evidence survives a later accounting rollback. It is not a
	// published result, settled debit or authority to repeat a paid POST.
	ObservedCostUSD  string `gorm:"size:128" json:"-"`
	EvidencePending  bool   `gorm:"not null;default:false" json:"-"`
	EvidenceVersion  int64  `gorm:"not null;default:0" json:"-"`
	BillingRevision  int64  `gorm:"not null;default:0" json:"-"`
	LogNextAttemptAt int64  `gorm:"not null;default:0;index:idx_async_jobs_log_due,priority:2" json:"-"`
	LogFailures      int    `gorm:"not null;default:0" json:"-"`
	ResultJSON       string `gorm:"size:1048576" json:"-"`
	ErrorCode        string `gorm:"size:64" json:"-"`
	LeaseOwner       string `gorm:"size:64" json:"-"`
	LeaseUntil       int64  `gorm:"index" json:"-"`
	NextPollAt       int64  `gorm:"index:idx_async_jobs_due,priority:2" json:"-"`
	PollFailures     int    `json:"-"`
	LogRecorded      bool   `gorm:"index:idx_async_jobs_log,priority:1;index:idx_async_jobs_log_due,priority:1" json:"-"`
	CreatedAt        int64  `gorm:"autoCreateTime:milli" json:"-"`
	UpdatedAt        int64  `gorm:"autoUpdateTime:milli;index:idx_async_jobs_log,priority:2" json:"-"`
	CompletedAt      int64  `gorm:"index" json:"-"`
}

// NewAsyncTaskID returns a random opaque gateway ID, not a provider task ID.
func NewAsyncTaskID() string { return "at_" + uuid.NewString() }

// AsyncTaskDedupKey scopes a caller-supplied idempotency key to an authenticated
// owner. Empty keys generate independent jobs. No raw key enters the database.
func AsyncTaskDedupKey(userID int, userUUID, key string) string {
	if key == "" {
		key = uuid.NewString()
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s", userID, userUUID, key)))
	return hex.EncodeToString(sum[:])
}

// AsyncTaskRequestHash canonicalizes JSON objects and hashes the original client
// model and parameters. Channel selection and sync/async delivery are deliberately
// excluded: retrying the same logical job can reattach through either interface.
func AsyncTaskRequestHash(body []byte) (string, error) {
	if len(body) == 0 || len(body) > MaxAsyncTaskBody {
		return "", errors.New("async video JSON exceeds the request limit")
	}
	var value map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil || value == nil {
		return "", errors.New("async video request must be a JSON object")
	}
	// Unmarshal validates trailing data that Decoder.Decode alone would accept.
	if !json.Valid(body) {
		return "", errors.New("invalid async video JSON")
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return "", errors.Wrap(err, "canonicalize async video request")
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

// FindAsyncTaskByDedup returns an owner-scoped existing task, nil when absent,
// or a conflict for a key reused with different inputs. UUID fences ID reuse.
func FindAsyncTaskByDedup(ctx context.Context, key, hash string, userID int, userUUID string) (*AsyncTask, error) {
	if DB == nil {
		return nil, errors.New("async task database unavailable")
	}
	var task AsyncTask
	err := DB.WithContext(ctx).Where("dedup_key = ? AND user_id = ? AND user_uuid = ?", key, userID, userUUID).Take(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "find async task receipt")
	}
	if subtle.ConstantTimeCompare([]byte(task.RequestHash), []byte(hash)) != 1 {
		return nil, ErrAsyncIdempotencyConflict
	}
	return &task, nil
}

// ReserveAsyncTask atomically records the input and reserves the FULL quoted
// quota. A unique receipt plus the wallet debit share one transaction, so a
// crash or concurrent idempotent retry cannot debit twice or create an orphan.
// Returns the persisted task and whether this call created it.
func ReserveAsyncTask(ctx context.Context, task *AsyncTask) (*AsyncTask, bool, error) {
	if DB == nil {
		return nil, false, errors.New("async task database unavailable")
	}
	if task == nil || task.UserID <= 0 || task.TokenID <= 0 || task.ChannelID <= 0 || task.Quota < 0 || task.Quota > math.MaxInt64/2 ||
		len(task.RequestBody) == 0 || len(task.RequestBody) > MaxAsyncTaskBody || len(task.DedupKey) != 64 || len(task.RequestHash) != 64 ||
		len(task.ActualModel) == 0 || len(task.ActualModel) > 128 || len(task.OriginModel) > 128 || len(task.BaseURL) > 2048 {
		return nil, false, errors.New("invalid async task reservation")
	}
	if existing, err := FindAsyncTaskByDedup(ctx, task.DedupKey, task.RequestHash, task.UserID, task.UserUUID); err != nil || existing != nil {
		return existing, false, err
	}
	// Reconstruct all server-owned lifecycle fields. Callers may pass an old
	// reservation snapshot, but cannot smuggle settled state, leases or receipts.
	*task = AsyncTask{ID: NewAsyncTaskID(), DedupKey: task.DedupKey, RequestHash: task.RequestHash,
		UserID: task.UserID, UserUUID: task.UserUUID, TokenID: task.TokenID, TokenUUID: task.TokenUUID,
		ChannelID: task.ChannelID, ChannelUUID: task.ChannelUUID, ChannelType: task.ChannelType,
		BaseURL: task.BaseURL, OriginModel: task.OriginModel, ActualModel: task.ActualModel,
		RequestBody: task.RequestBody, RequestID: task.RequestID, TraceID: task.TraceID,
		Quota: task.Quota, QuotedQuota: task.Quota, CostQuotaPerUSD: task.CostQuotaPerUSD, BillingRevision: 1}
	if task.CostQuotaPerUSD != "" {
		if _, err := AsyncUpstreamCostQuota(task.CostQuotaPerUSD, "0"); err != nil {
			return nil, false, errors.Wrap(err, "invalid async pricing snapshot")
		}
	}
	task.State, task.BillingState = AsyncTaskReserved, AsyncBillingHeld
	task.CreatedAt = time.Now().UTC().UnixMilli()
	task.UpdatedAt, task.NextPollAt = task.CreatedAt, task.CreatedAt
	var token Token
	err := runWithSQLiteBusyRetryForDB(ctx, DB, func() error {
		return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("id = ? AND user_id = ?", task.TokenID, task.UserID).Take(&token).Error; err != nil {
				return errors.Wrap(err, "load async task token")
			}
			var user User
			if err := tx.Where("id = ?", task.UserID).Take(&user).Error; err != nil {
				return errors.Wrap(err, "load async task owner")
			}
			if token.UUID != task.TokenUUID || user.UUID != task.UserUUID || user.Status != UserStatusEnabled ||
				token.Status != TokenStatusEnabled || (token.ExpiredTime != -1 && token.ExpiredTime < time.Now().Unix()) {
				return errors.New("async task owner or token is unavailable")
			}
			if user.Quota < task.Quota || (!token.UnlimitedQuota && (token.RemainQuota < task.Quota || token.UsedQuota > math.MaxInt64-task.Quota)) {
				return ErrAsyncQuota
			}
			task.TokenUnlimited, task.TokenName = token.UnlimitedQuota, token.Name
			if err := tx.Create(task).Error; err != nil {
				return errors.Wrap(err, "insert async task reservation")
			}
			if task.Quota > 0 {
				if err := adjustAsyncTaskQuota(tx, task, task.Quota, true); err != nil {
					return err
				}
			}
			return nil
		})
	})
	if err != nil {
		// A concurrent creator (or a lost COMMIT acknowledgement) may have committed
		// the same receipt. Read on a fresh transaction, including PostgreSQL.
		existing, lookupErr := FindAsyncTaskByDedup(ctx, task.DedupKey, task.RequestHash, task.UserID, task.UserUUID)
		if lookupErr != nil {
			return nil, false, lookupErr
		}
		if existing != nil {
			return existing, false, nil
		}
		return nil, false, errors.Wrap(err, "reserve asynchronous task")
	}
	refreshAsyncTaskQuotaCache(ctx, &token)
	return task, true, nil
}

// GetOwnedAsyncTask reads a gateway task without requiring a live channel or
// positive balance. Authorization is based on owner ID AND immutable UUID.
func GetOwnedAsyncTask(ctx context.Context, id string, userID int, userUUID string) (*AsyncTask, error) {
	if DB == nil {
		return nil, errors.New("async task database unavailable")
	}
	var task AsyncTask
	err := DB.WithContext(ctx).Where("id = ? AND user_id = ? AND user_uuid = ?", id, userID, userUUID).Take(&task).Error
	return &task, errors.Wrap(err, "get owned async task")
}

// refreshAsyncTaskQuotaCache invalidates advisory caches after committed wallet
// changes. Cache failures never roll back or replay a durable financial operation.
func refreshAsyncTaskQuotaCache(ctx context.Context, token *Token) {
	if token.Key != "" {
		clearTokenCache(ctx, token.Key)
	}
	if err := CacheUpdateUserQuota(ctx, token.UserId); err != nil {
		logger.FromContext(ctx).Warn("async task quota cache refresh failed", zap.Error(err))
	}
}
