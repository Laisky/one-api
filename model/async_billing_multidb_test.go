package model

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
)

// asyncBillingLiveStore creates unique, disposable primary/log databases on a
// configured test backend. It never drops or migrates the DSN's original DB.
// CI requires BOTH engines explicitly; missing DSNs are not accepted as passes.
func asyncBillingLiveStore(t *testing.T, backend string) *AsyncTask {
	t.Helper()
	env := "MYSQL_DSN"
	if backend == "postgres" {
		env = "PG_DSN"
	}
	dsn := os.Getenv(env)
	if dsn == "" {
		if os.Getenv("ONEAPI_REQUIRE_DB_BACKENDS") == "1" {
			t.Fatalf("%s required", env)
		}
		t.Skip(env + " not configured")
	}
	open := func(value string) *gorm.DB {
		var dial gorm.Dialector
		if backend == "mysql" {
			dial = mysql.Open(value)
		} else {
			dial = postgres.Open(value)
		}
		db, err := gorm.Open(dial, &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
		require.NoError(t, err)
		conn, err := db.DB()
		require.NoError(t, err)
		conn.SetMaxOpenConns(16)
		t.Cleanup(func() { require.NoError(t, conn.Close()) })
		return db
	}
	admin := open(dsn)
	create := func() *gorm.DB {
		name := "asyncbill_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		require.NoError(t, admin.Exec("CREATE DATABASE "+name).Error)
		t.Cleanup(func() { require.NoError(t, admin.Exec("DROP DATABASE "+name).Error) })
		value := dsn
		if backend == "mysql" {
			cfg, err := mysqldriver.ParseDSN(dsn)
			require.NoError(t, err)
			cfg.DBName = name
			value = cfg.FormatDSN()
		} else {
			if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
				parsed, err := url.Parse(dsn)
				require.NoError(t, err)
				parsed.Path = "/" + name
				value = parsed.String()
			} else {
				value = dsn + " dbname=" + name
			}
		}
		return open(value)
	}
	primary, logDB := create(), create()
	oldDB, oldLog := DB, LOG_DB
	oldRedis, oldSQLite, oldMysql, oldPg, oldLogging := common.IsRedisEnabled(), common.UsingSQLite.Load(), common.UsingMySQL.Load(), common.UsingPostgreSQL.Load(), config.IsLogConsumeEnabled()
	DB, LOG_DB = primary, logDB
	common.SetRedisEnabled(false)
	common.UsingSQLite.Store(false)
	common.UsingMySQL.Store(backend == "mysql")
	common.UsingPostgreSQL.Store(backend == "postgres")
	config.SetLogConsumeEnabled(true)
	t.Cleanup(func() {
		DB, LOG_DB = oldDB, oldLog
		common.SetRedisEnabled(oldRedis)
		common.UsingSQLite.Store(oldSQLite)
		common.UsingMySQL.Store(oldMysql)
		common.UsingPostgreSQL.Store(oldPg)
		config.SetLogConsumeEnabled(oldLogging)
	})
	require.NoError(t, primary.AutoMigrate(&User{}, &Token{}, &Channel{}, &AsyncTask{}, &UserRequestCost{}))
	require.NoError(t, logDB.AutoMigrate(&Log{}, &AsyncTaskLogReceipt{}))
	user := User{Id: 1001, UUID: uuid.NewString(), Username: "billing-live", Status: UserStatusEnabled, Quota: 1000000}
	token := Token{Id: 1002, UUID: uuid.NewString(), UserId: user.Id, Key: uuid.NewString(), Status: TokenStatusEnabled, RemainQuota: 1000000, ExpiredTime: -1}
	channel := Channel{Id: 1003, UUID: uuid.NewString(), Type: 61, Name: "live-fixture", Status: ChannelStatusEnabled}
	require.NoError(t, primary.Create(&user).Error)
	require.NoError(t, primary.Create(&token).Error)
	require.NoError(t, primary.Create(&channel).Error)
	body := `{"model":"veo3-fast","duration":5}`
	hash, err := AsyncTaskRequestHash([]byte(body))
	require.NoError(t, err)
	return &AsyncTask{UserID: user.Id, UserUUID: user.UUID, TokenID: token.Id, TokenUUID: token.UUID, ChannelID: channel.Id, ChannelUUID: channel.UUID, ChannelType: channel.Type, BaseURL: "https://provider.example", OriginModel: "veo3-fast", ActualModel: "veo3-fast", RequestBody: body, RequestHash: hash, DedupKey: AsyncTaskDedupKey(user.Id, user.UUID, "live-key"), RequestID: "live-request", Quota: 200000, CostQuotaPerUSD: "500000"}
}

// TestAsyncBillingLiveDatabases exercises real MySQL/PostgreSQL transactions,
// concurrent idempotent admission, competing settlement/refund, negative debt,
// mutable token policy, and split-store versioned logs. Nothing substitutes
// SQLite dialect SQL or a mock transaction for either required backend.
func TestAsyncBillingLiveDatabases(t *testing.T) {
	for _, backend := range []string{"mysql", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			for _, unlimited := range []bool{false, true} {
				t.Run(fmt.Sprintf("unlimited=%t", unlimited), func(t *testing.T) {
					template := asyncBillingLiveStore(t, backend)
					require.NoError(t, DB.Model(&Token{}).Where("id = ?", template.TokenID).Update("unlimited_quota", unlimited).Error)
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					const concurrency = 12
					results := make(chan *AsyncTask, concurrency)
					errs := make(chan error, concurrency)
					start := make(chan struct{})
					var wg sync.WaitGroup
					for range concurrency {
						wg.Add(1)
						go func() {
							defer wg.Done()
							<-start
							input := *template
							task, _, err := ReserveAsyncTask(ctx, &input)
							results <- task
							errs <- err
						}()
					}
					close(start)
					wg.Wait()
					close(results)
					close(errs)
					for err := range errs {
						require.NoError(t, err)
					}
					var task *AsyncTask
					for item := range results {
						require.NotNil(t, item)
						if task == nil {
							task = item
						}
						require.Equal(t, task.ID, item.ID)
					}
					user, token, _ := asyncBillingBalances(t, task)
					require.EqualValues(t, 800000, user.Quota)
					if unlimited {
						require.EqualValues(t, 1000000, token.RemainQuota)
					} else {
						require.EqualValues(t, 800000, token.RemainQuota)
					}
					require.NoError(t, FlushAsyncTaskLogs(ctx))
					stale := *task
					lease, err := ClaimAsyncTask(ctx, time.Now())
					require.NoError(t, err)
					require.NotNil(t, lease)
					require.NoError(t, ApplyAsyncTaskUpdate(ctx, lease, AsyncTaskUpdate{State: AsyncTaskQueued, UpstreamID: "live-job"}))
					// The original token mode, not an administrator's subsequent toggle, owns
					// refunds and supplements for this already admitted task.
					require.NoError(t, DB.Model(&Token{}).Where("id = ?", template.TokenID).Update("unlimited_quota", !unlimited).Error)
					lease, err = ClaimAsyncTask(ctx, time.Now())
					require.NoError(t, err)
					require.NotNil(t, lease)
					require.NoError(t, ApplyAsyncTaskUpdate(ctx, lease, AsyncTaskUpdate{State: AsyncTaskCompleted, CostUSD: "3.0", ResultJSON: `{"videos":[{"url":"https://media.example/v.mp4"}]}`}))
					require.ErrorIs(t, ApplyAsyncTaskUpdate(ctx, lease, AsyncTaskUpdate{State: AsyncTaskFailed, Refund: true}), ErrAsyncLeaseLost)
					require.NoError(t, FlushAsyncTaskLogs(ctx))
					require.NoError(t, writeAsyncTaskRequestCost(ctx, &stale))
					require.NoError(t, writeAsyncTaskLog(ctx, &stale))
					require.NoError(t, FlushAsyncTaskLogs(ctx))
					user, token, channel := asyncBillingBalances(t, task)
					require.EqualValues(t, -500000, user.Quota)
					require.EqualValues(t, 1500000, user.UsedQuota)
					require.EqualValues(t, 1500000, channel.UsedQuota)
					require.EqualValues(t, 1, user.RequestCount)
					if unlimited {
						require.EqualValues(t, 1000000, token.RemainQuota)
					} else {
						require.EqualValues(t, -500000, token.RemainQuota)
						require.EqualValues(t, 1500000, token.UsedQuota)
					}
					var logs []Log
					require.NoError(t, LOG_DB.Find(&logs).Error)
					require.Len(t, logs, 1)
					require.EqualValues(t, 1500000, logs[0].Quota)
					var cost UserRequestCost
					require.NoError(t, DB.Where("request_id = ?", task.RequestID).Take(&cost).Error)
					require.EqualValues(t, 1500000, cost.Quota)
				})
			}
			t.Run("competing-refunds", func(t *testing.T) {
				task := reserveDurableJob(t, asyncBillingLiveStore(t, backend))
				ctx := context.Background()
				lease, err := ClaimAsyncTask(ctx, time.Now())
				require.NoError(t, err)
				require.NotNil(t, lease)
				start := make(chan struct{})
				errs := make(chan error, 16)
				var wg sync.WaitGroup
				for range 16 {
					wg.Add(1)
					go func() {
						defer wg.Done()
						<-start
						copy := *lease
						errs <- ApplyAsyncTaskUpdate(ctx, &copy, AsyncTaskUpdate{State: AsyncTaskFailed, Refund: true})
					}()
				}
				close(start)
				wg.Wait()
				close(errs)
				wins := 0
				for err := range errs {
					if err == nil {
						wins++
					} else {
						require.ErrorIs(t, err, ErrAsyncLeaseLost)
					}
				}
				require.Equal(t, 1, wins)
				user, token, channel := asyncBillingBalances(t, task)
				require.EqualValues(t, 1000000, user.Quota)
				require.EqualValues(t, 1000000, token.RemainQuota)
				require.Zero(t, token.UsedQuota)
				require.Zero(t, channel.UsedQuota)
				require.NoError(t, FlushAsyncTaskLogs(ctx))
				var logs []Log
				require.NoError(t, LOG_DB.Find(&logs).Error)
				require.Len(t, logs, 1)
				require.Zero(t, logs[0].Quota)
			})
		})
	}
}
