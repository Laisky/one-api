package asyncvideo_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/muapi"
	"github.com/Laisky/one-api/relay/asyncvideo"
	"github.com/Laisky/one-api/relay/meta"
)

// openCrashBillingStore uses independent on-disk primary/log connections in each
// process. SQLite's real rollback journal, not an in-memory state machine, owns
// recovery when the test kills a process during a financial transaction.
func openCrashBillingStore(t *testing.T, dir string) {
	t.Helper()
	oldDB, oldLog := model.DB, model.LOG_DB
	oldRedis, oldSQLite, oldLogging := common.IsRedisEnabled(), common.UsingSQLite.Load(), config.IsLogConsumeEnabled()
	open := func(name string) *gorm.DB {
		db, err := gorm.Open(sqlite.Open(filepath.Join(dir, name)+"?_busy_timeout=5000"), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
		require.NoError(t, err)
		conn, err := db.DB()
		require.NoError(t, err)
		conn.SetMaxOpenConns(1)
		t.Cleanup(func() { require.NoError(t, conn.Close()) })
		return db
	}
	model.DB, model.LOG_DB = open("tasks.db"), open("logs.db")
	common.SetRedisEnabled(false)
	common.UsingSQLite.Store(true)
	config.SetLogConsumeEnabled(true)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLog
		common.SetRedisEnabled(oldRedis)
		common.UsingSQLite.Store(oldSQLite)
		config.SetLogConsumeEnabled(oldLogging)
	})
}

// crashBillingResolver connects only to the local HTTP fixture supplied by the
// parent. The immutable provider identity/body still come from the durable task.
func crashBillingResolver(base string) asyncvideo.Resolver {
	return func(context.Context, *model.AsyncTask) (asyncvideo.Provider, *meta.Meta, error) {
		return &muapi.Adaptor{}, &meta.Meta{BaseURL: base, ActualModelName: "veo3-fast", APIKey: "fixture-only"}, nil
	}
}

// TestAsyncBillingProcessChild is a separately executable worker entry point.
// The parent kills it at a deterministic I/O/transaction boundary, so no deferred
// refund, goroutine cleanup or graceful shutdown can mask a lost debit.
func TestAsyncBillingProcessChild(t *testing.T) {
	dir := os.Getenv("ONEAPI_ASYNC_BILLING_CHILD_DIR")
	if dir == "" {
		t.Skip("subprocess entry point")
	}
	stage, base := os.Getenv("ONEAPI_ASYNC_BILLING_CHILD_STAGE"), os.Getenv("ONEAPI_ASYNC_BILLING_CHILD_URL")
	openCrashBillingStore(t, dir)
	pause := func() {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "ready"), []byte(stage), 0600))
		time.Sleep(30 * time.Second)
		t.Fatal("parent did not kill worker")
	}
	if stage == "reserved" {
		pause()
		return
	}
	if stage == "before_settlement_commit" || stage == "during_additional_debit" || stage == "outbox_before_ack" || stage == "submit_debit_before_commit" {
		require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register("test:crash_boundary", func(tx *gorm.DB) {
			fields, ok := tx.Statement.Dest.(map[string]any)
			if !ok {
				return
			}
			if stage == "before_settlement_commit" && tx.Statement.Table == "async_tasks" && fields["state"] == model.AsyncTaskCompleted {
				pause()
			}
			if (stage == "during_additional_debit" || stage == "submit_debit_before_commit") && tx.Statement.Table == "tokens" {
				pause()
			}
			if stage == "outbox_before_ack" && tx.Statement.Table == "async_tasks" && fields["log_recorded"] == true {
				pause()
			}
		}))
	}
	step := func() {
		worked, err := asyncvideo.ProcessOne(context.Background(), crashBillingResolver(base), time.Now())
		require.NoError(t, err)
		require.True(t, worked)
	}
	step()
	if stage == "accepted" {
		pause()
		return
	}
	require.NoError(t, model.DB.Model(&model.AsyncTask{}).Where("billing_state = ?", model.AsyncBillingHeld).Update("next_poll_at", 0).Error)
	step()
	if stage == "outbox_before_ack" {
		require.NoError(t, model.FlushAsyncTaskLogs(context.Background()))
	}
	pause()
}

// TestAsyncBillingSIGKILLRecovery verifies physical balances, task state, request
// cost and one log after SIGKILL at nine worker/ledger boundaries. An ambiguous
// paid POST stays charged and is never automatically retried; known jobs resume
// GET polling, collect final cost, and survive split-database acknowledgement loss.
func TestAsyncBillingSIGKILLRecovery(t *testing.T) {
	stages := []string{"reserved", "during_submission", "accepted", "during_poll", "before_settlement_commit", "during_additional_debit", "settled", "outbox_before_ack", "submit_debit_before_commit"}
	for _, stage := range stages {
		t.Run(stage, func(t *testing.T) {
			dir := t.TempDir()
			openCrashBillingStore(t, dir)
			require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.AsyncTask{}, &model.UserRequestCost{}))
			require.NoError(t, model.LOG_DB.AutoMigrate(&model.Log{}, &model.AsyncTaskLogReceipt{}))
			user := model.User{Id: 901, UUID: uuid.NewString(), Username: "crash-owner", Status: model.UserStatusEnabled, Quota: 200000}
			token := model.Token{Id: 902, UUID: uuid.NewString(), UserId: user.Id, Key: uuid.NewString(), Status: model.TokenStatusEnabled, RemainQuota: 200000, ExpiredTime: -1}
			channel := model.Channel{Id: 903, UUID: uuid.NewString(), Type: 61, Status: model.ChannelStatusEnabled, Name: "crash-provider"}
			require.NoError(t, model.DB.Create(&user).Error)
			require.NoError(t, model.DB.Create(&token).Error)
			require.NoError(t, model.DB.Create(&channel).Error)
			var creates, polls atomic.Int32
			blocked := make(chan struct{}, 1)
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.Copy(io.Discard, r.Body)
				if r.Method == http.MethodPost {
					creates.Add(1)
					if stage == "during_submission" {
						blocked <- struct{}{}
						<-r.Context().Done()
						return
					}
					if stage == "submit_debit_before_commit" {
						_, _ = io.WriteString(w, `{"request_id":"crash-job","cost":{"amount_usd":0.8}}`)
						return
					}
					_, _ = io.WriteString(w, `{"request_id":"crash-job"}`)
					return
				}
				if polls.Add(1) == 1 && stage == "during_poll" {
					blocked <- struct{}{}
					<-r.Context().Done()
					return
				}
				if stage == "submit_debit_before_commit" {
					// The replacement worker must rely on the pre-crash charge,
					// not a cost field helpfully repeated by the provider.
					_, _ = io.WriteString(w, `{"id":"crash-job","status":"completed","outputs":["https://media.example/crash.mp4"]}`)
					return
				}
				_, _ = io.WriteString(w, `{"id":"crash-job","status":"completed","outputs":["https://media.example/crash.mp4"],"cost":{"amount_usd":0.8,"refunded":false}}`)
			}))
			defer provider.Close()
			body := `{"duration":5,"prompt":"crash fixture"}`
			hash, err := model.AsyncTaskRequestHash([]byte(body))
			require.NoError(t, err)
			task := &model.AsyncTask{UserID: user.Id, UserUUID: user.UUID, TokenID: token.Id, TokenUUID: token.UUID, ChannelID: channel.Id, ChannelUUID: channel.UUID, ChannelType: channel.Type, OriginModel: "veo3-fast", ActualModel: "veo3-fast", BaseURL: provider.URL, Quota: 200000, CostQuotaPerUSD: "500000", RequestBody: body, RequestHash: hash, RequestID: "crash-request", DedupKey: model.AsyncTaskDedupKey(user.Id, user.UUID, "crash-fixture")}
			saved, created, err := model.ReserveAsyncTask(context.Background(), task)
			require.NoError(t, err)
			require.True(t, created)
			binary, err := os.Executable()
			require.NoError(t, err)
			deadline, stop := context.WithTimeout(context.Background(), 20*time.Second)
			defer stop()
			command := exec.CommandContext(deadline, binary, "-test.run=^TestAsyncBillingProcessChild$", "-test.timeout=25s")
			command.Env = append(os.Environ(), "ONEAPI_ASYNC_BILLING_CHILD_DIR="+dir, "ONEAPI_ASYNC_BILLING_CHILD_STAGE="+stage, "ONEAPI_ASYNC_BILLING_CHILD_URL="+provider.URL)
			log, err := os.Create(filepath.Join(dir, "child.log"))
			require.NoError(t, err)
			defer log.Close()
			command.Stdout, command.Stderr = log, log
			require.NoError(t, command.Start())
			waited := false
			t.Cleanup(func() {
				if !waited {
					_ = command.Process.Kill()
					_ = command.Wait()
				}
			})
			if strings.HasPrefix(stage, "during_sub") || stage == "during_poll" {
				select {
				case <-blocked:
				case <-time.After(15 * time.Second):
					data, _ := os.ReadFile(filepath.Join(dir, "child.log"))
					t.Fatalf("worker did not reach provider boundary: %s", data)
				}
			} else {
				require.Eventually(t, func() bool { _, err := os.Stat(filepath.Join(dir, "ready")); return err == nil }, 15*time.Second, 10*time.Millisecond, "child boundary %s", stage)
			}
			require.NoError(t, command.Process.Kill())
			require.Error(t, command.Wait())
			waited = true
			// Only the clock is advanced: this simulates lease expiration, not a task
			// outcome or refund. Provider calls and SQL balances remain the real ones.
			require.NoError(t, model.DB.Model(&model.AsyncTask{}).Where("id = ?", saved.ID).Updates(map[string]any{"lease_until": 0, "next_poll_at": 0}).Error)
			for range 3 {
				_, err := asyncvideo.ProcessOne(context.Background(), crashBillingResolver(provider.URL), time.Now())
				require.NoError(t, err)
				require.NoError(t, model.DB.Model(&model.AsyncTask{}).Where("id = ?", saved.ID).Update("next_poll_at", 0).Error)
			}
			require.NoError(t, model.FlushAsyncTaskLogs(context.Background()))
			require.NoError(t, model.FlushAsyncTaskLogs(context.Background()))
			var final model.AsyncTask
			require.NoError(t, model.DB.Where("id = ?", saved.ID).Take(&final).Error)
			require.NoError(t, model.DB.Where("id = ?", user.Id).Take(&user).Error)
			require.NoError(t, model.DB.Where("id = ?", token.Id).Take(&token).Error)
			require.NoError(t, model.DB.Where("id = ?", channel.Id).Take(&channel).Error)
			expected := int64(400000)
			if stage == "during_submission" {
				expected = 200000
				require.Equal(t, model.AsyncTaskUnknown, final.State)
				require.Equal(t, model.AsyncBillingHeld, final.BillingState)
				require.Zero(t, user.UsedQuota)
				require.Zero(t, channel.UsedQuota)
				require.Zero(t, polls.Load())
			} else {
				require.Equal(t, model.AsyncTaskCompleted, final.State)
				require.Equal(t, model.AsyncBillingSettled, final.BillingState)
				require.EqualValues(t, expected, user.UsedQuota)
				require.EqualValues(t, expected, channel.UsedQuota)
				require.EqualValues(t, 1, user.RequestCount)
			}
			require.EqualValues(t, 1, creates.Load(), "no crash may replay a paid POST")
			require.EqualValues(t, 200000-expected, user.Quota)
			require.EqualValues(t, 200000-expected, token.RemainQuota)
			require.EqualValues(t, expected, token.UsedQuota)
			require.EqualValues(t, expected, final.Quota)
			var cost model.UserRequestCost
			require.NoError(t, model.DB.Where("request_id = ?", task.RequestID).Take(&cost).Error)
			require.EqualValues(t, expected, cost.Quota)
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Find(&logs).Error)
			require.Len(t, logs, 1)
			require.EqualValues(t, expected, logs[0].Quota)
		})
	}
}
