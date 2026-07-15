package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// uuidWriterOwnedCase creates one owned-UUID row through its production ORM writer path and
// returns the UUID the create hook assigned.
type uuidWriterOwnedCase func(t *testing.T, db *gorm.DB) string

// uuidWriterOwnedCases maps every owned UUID table to a create that exercises its hook.
// TestUUIDAwareCreatePathsPopulateEveryOwnedUUID proves this key set equals uuidOwnedRegistry's
// table set, so a newly added owned table cannot silently escape the writer contract.
// Parameters: none.
//
// Return values:
//   - map[string]uuidWriterOwnedCase: owned table name to ORM create.
func uuidWriterOwnedCases() map[string]uuidWriterOwnedCase {
	return map[string]uuidWriterOwnedCase{
		"users": func(t *testing.T, db *gorm.DB) string {
			row := &User{Username: "owned-user", Password: "password-hash",
				AccessToken: "owned-user-access-token", AffCode: "own1"}
			require.NoError(t, db.Create(row).Error)
			return row.UUID
		},
		"tokens": func(t *testing.T, db *gorm.DB) string {
			row := &Token{UserId: 1, Key: "owned-token-key", Name: "owned-token"}
			require.NoError(t, db.Create(row).Error)
			return row.UUID
		},
		"channels": func(t *testing.T, db *gorm.DB) string {
			row := &Channel{Type: 1, Name: "owned-channel", Models: "gpt-4o", Config: "{}"}
			require.NoError(t, db.Create(row).Error)
			return row.UUID
		},
		"redemptions": func(t *testing.T, db *gorm.DB) string {
			row := &Redemption{UserId: 1, Key: "owned-redemption-key", Name: "owned-gift"}
			require.NoError(t, db.Create(row).Error)
			return row.UUID
		},
		"token_transactions": func(t *testing.T, db *gorm.DB) string {
			row := &TokenTransaction{TransactionID: "owned-txn", TokenId: 1, UserId: 1,
				Status: TokenTransactionStatusPending, PreQuota: 10}
			require.NoError(t, db.Create(row).Error)
			return row.UUID
		},
		"user_request_costs": func(t *testing.T, db *gorm.DB) string {
			row := &UserRequestCost{UserID: 1, RequestID: "owned-request", Quota: 10}
			require.NoError(t, db.Create(row).Error)
			return row.UUID
		},
		"traces": func(t *testing.T, db *gorm.DB) string {
			row := &Trace{TraceId: "owned-trace", URL: "/v1/chat/completions", Method: "POST",
				Timestamps: `{"request_received":1}`}
			require.NoError(t, db.Create(row).Error)
			return row.UUID
		},
		"async_task_bindings": func(t *testing.T, db *gorm.DB) string {
			row := &AsyncTaskBinding{TaskID: "owned-task", TaskType: "video", UserID: 1,
				TokenID: 1, ChannelID: 1, ChannelType: 1}
			require.NoError(t, db.Create(row).Error)
			return row.UUID
		},
		"mcp_servers": func(t *testing.T, db *gorm.DB) string {
			row := &MCPServer{Name: "owned-server", BaseURL: "https://example.com/mcp",
				Protocol: MCPProtocolStreamableHTTP, AuthType: MCPAuthTypeNone}
			require.NoError(t, db.Create(row).Error)
			return row.UUID
		},
		"mcp_tools": func(t *testing.T, db *gorm.DB) string {
			row := &MCPTool{ServerId: 1, Name: "owned-tool"}
			require.NoError(t, db.Create(row).Error)
			return row.UUID
		},
		"passkey_credentials": func(t *testing.T, db *gorm.DB) string {
			row := &PasskeyCredential{UserId: 1, CredentialName: "owned-passkey",
				CredentialID: []byte("owned-credential-id"), PublicKey: []byte("owned-public-key")}
			require.NoError(t, db.Create(row).Error)
			return row.UUID
		},
		"logs": func(t *testing.T, db *gorm.DB) string {
			row := &Log{UserId: 1, Type: LogTypeSystem, Content: "owned log"}
			require.NoError(t, db.Create(row).Error)
			return row.UUID
		},
	}
}

// TestUUIDAwareCreatePathsPopulateEveryOwnedUUID covers UUID-A22: every owned UUID table in
// uuidOwnedRegistry has a create path whose hook writes a canonical hyphenated UUIDv7, so a
// UUID-aware writer never produces the missing owned UUID that invariant 5 makes a hard
// finalization blocker. The coverage is driven from the registry itself: a registry table with
// no create case fails here rather than silently escaping the contract.
func TestUUIDAwareCreatePathsPopulateEveryOwnedUUID(t *testing.T) {
	db, _ := newUnifiedTestTopology(t)
	cases := uuidWriterOwnedCases()

	registryTables := map[string]bool{}
	for _, target := range uuidOwnedRegistry() {
		registryTables[target.table] = true
		require.Containsf(t, cases, target.table,
			"owned registry table %s has no UUID-aware create case", target.table)
	}
	for table := range cases {
		require.Containsf(t, registryTables, table,
			"create case %s is not an owned registry table", table)
	}
	require.Len(t, cases, len(uuidOwnedRegistry()), "the registry lists 12 owned UUID tables")

	// A UUID that repeats across tables would still satisfy each table's unique index while
	// breaking the external identifier contract, so distinctness is asserted globally.
	assigned := map[string]string{}
	for _, target := range uuidOwnedRegistry() {
		target := target
		t.Run(target.table, func(t *testing.T) {
			assignedUUID := cases[target.table](t, db)
			require.NotEmpty(t, assignedUUID, "%s.uuid must be assigned by the create hook", target.table)
			requireHyphenatedUUID(t, assignedUUID)

			// The in-memory value the hook produced must be the value that actually landed in
			// the authoritative column; a hook that only mutates the struct would still leave a
			// finalization-blocking gap in the table.
			var stored int64
			require.NoError(t, db.Table(target.table).Where("uuid = ?", assignedUUID).Count(&stored).Error)
			require.EqualValues(t, 1, stored,
				"the hook's uuid must be persisted in %s.uuid", target.table)

			require.NotContainsf(t, assigned, assignedUUID,
				"%s reused the uuid already assigned to %s", target.table, assigned[assignedUUID])
			assigned[assignedUUID] = target.table
		})
	}
	require.Len(t, assigned, len(uuidOwnedRegistry()))
}

// uuidWriterFixture holds the owner rows that a denormalized FK UUID writer copies from.
type uuidWriterFixture struct {
	// inviter owns users.inviter_uuid.
	inviter *User
	// user owns every user_uuid column.
	user *User
	// channel owns every channel_uuid column.
	channel *Channel
	// token owns every token_uuid column.
	token *Token
	// server owns mcp_tools.server_uuid.
	server *MCPServer
	// logRow owns token_transactions.log_uuid.
	logRow *Log
}

// uuidWriterFKCase documents which code path owns one denormalized FK UUID column and records
// the honest current behavior of the writer that this package can exercise.
type uuidWriterFKCase struct {
	// responsible names the code path contractually required to populate the column.
	responsible string
	// populatedByModelWriter is true when the model-package writer exercised by write fills the
	// column itself. When false the column is only ever populated by an explicit call site
	// outside package model, and the test asserts that honest behavior rather than a fiction.
	populatedByModelWriter bool
	// write runs the writer path and returns the FK UUID value that was persisted.
	write func(t *testing.T, db *gorm.DB, fixture *uuidWriterFixture) *string
	// wantUUID returns the owner UUID the column must carry when populatedByModelWriter is true.
	wantUUID func(fixture *uuidWriterFixture) string
}

// uuidWriterFKCases maps every denormalized FK UUID column to its responsible writer path.
// Parameters: none.
//
// Return values:
//   - map[string]uuidWriterFKCase: "table.column" to writer contract.
func uuidWriterFKCases() map[string]uuidWriterFKCase {
	return map[string]uuidWriterFKCase{
		"users.inviter_uuid": {
			responsible:            "model.(*User).Insert resolves the inviter uuid before DB.Create",
			populatedByModelWriter: true,
			wantUUID:               func(fixture *uuidWriterFixture) string { return fixture.inviter.UUID },
			write: func(t *testing.T, db *gorm.DB, fixture *uuidWriterFixture) *string {
				invitee := &User{Username: "fk-invitee", Password: "password-hash"}
				require.NoError(t, invitee.Insert(context.Background(), fixture.inviter.Id))
				stored := &User{}
				require.NoError(t, db.First(stored, "id = ?", invitee.Id).Error)
				return stored.InviterUUID
			},
		},
		"tokens.user_uuid": {
			responsible:            "model.(*Token).Insert resolves the owner uuid before DB.Create",
			populatedByModelWriter: true,
			wantUUID:               func(fixture *uuidWriterFixture) string { return fixture.user.UUID },
			write: func(t *testing.T, db *gorm.DB, fixture *uuidWriterFixture) *string {
				token := &Token{UserId: fixture.user.Id, Key: "fk-token-key", Name: "fk-token"}
				require.NoError(t, token.Insert(context.Background()))
				stored := &Token{}
				require.NoError(t, db.First(stored, "id = ?", token.Id).Error)
				return stored.UserUUID
			},
		},
		"redemptions.user_uuid": {
			// FINDING: model.(*Redemption).Insert is a bare DB.Create. controller.AddRedemption
			// sets UserUUID from ctxkey.UserUUID before calling it, so any other caller of
			// Insert leaves a gap that only catch-up can close.
			responsible:            "controller.AddRedemption sets Redemption.UserUUID before model.(*Redemption).Insert",
			populatedByModelWriter: false,
			write: func(t *testing.T, db *gorm.DB, fixture *uuidWriterFixture) *string {
				redemption := &Redemption{UserId: fixture.user.Id, Key: "fk-redemption-key", Name: "fk-gift"}
				require.NoError(t, redemption.Insert())
				stored := &Redemption{}
				require.NoError(t, db.First(stored, "id = ?", redemption.Id).Error)
				return stored.UserUUID
			},
		},
		"token_transactions.token_uuid": {
			// FINDING: model.CreateTokenTransaction persists exactly what the caller built.
			responsible:            "controller/token.go builds TokenTransaction.TokenUUID before model.CreateTokenTransaction",
			populatedByModelWriter: false,
			write: func(t *testing.T, db *gorm.DB, fixture *uuidWriterFixture) *string {
				return writeUUIDFKTokenTransaction(t, db, fixture, "fk-txn-token-uuid",
					func(stored *TokenTransaction) *string { return stored.TokenUUID })
			},
		},
		"token_transactions.user_uuid": {
			responsible:            "controller/token.go builds TokenTransaction.UserUUID before model.CreateTokenTransaction",
			populatedByModelWriter: false,
			write: func(t *testing.T, db *gorm.DB, fixture *uuidWriterFixture) *string {
				return writeUUIDFKTokenTransaction(t, db, fixture, "fk-txn-user-uuid",
					func(stored *TokenTransaction) *string { return stored.UserUUID })
			},
		},
		"token_transactions.log_uuid": {
			// FINDING: log_uuid has the narrowest writer of all 15 columns. Only the two
			// consume paths in controller/token.go ever assign it, from the log row they just
			// wrote; nothing in package model does.
			responsible:            "controller/token.go assigns transaction.LogUUID from the consume log it just recorded",
			populatedByModelWriter: false,
			write: func(t *testing.T, db *gorm.DB, fixture *uuidWriterFixture) *string {
				return writeUUIDFKTokenTransaction(t, db, fixture, "fk-txn-log-uuid",
					func(stored *TokenTransaction) *string { return stored.LogUUID })
			},
		},
		"user_request_costs.user_uuid": {
			responsible:            "model.(*UserRequestCost).Insert and model.UpdateUserRequestCostQuotaByRequestID resolve the owner uuid",
			populatedByModelWriter: true,
			wantUUID:               func(fixture *uuidWriterFixture) string { return fixture.user.UUID },
			write: func(t *testing.T, db *gorm.DB, fixture *uuidWriterFixture) *string {
				cost := NewUserRequestCost(fixture.user.Id, "fk-request-id", 10)
				require.NoError(t, cost.Insert())
				stored := &UserRequestCost{}
				require.NoError(t, db.First(stored, "id = ?", cost.Id).Error)
				return stored.UserUUID
			},
		},
		"async_task_bindings.user_uuid": {
			// FINDING: model.SaveAsyncTaskBinding validates and persists but resolves no uuid.
			responsible:            "relay adaptors populate AsyncTaskBinding.UserUUID before model.SaveAsyncTaskBinding",
			populatedByModelWriter: false,
			write: func(t *testing.T, db *gorm.DB, fixture *uuidWriterFixture) *string {
				return writeUUIDFKAsyncTaskBinding(t, db, fixture, "fk-task-user-uuid",
					func(stored *AsyncTaskBinding) *string { return stored.UserUUID })
			},
		},
		"async_task_bindings.token_uuid": {
			responsible:            "relay adaptors populate AsyncTaskBinding.TokenUUID before model.SaveAsyncTaskBinding",
			populatedByModelWriter: false,
			write: func(t *testing.T, db *gorm.DB, fixture *uuidWriterFixture) *string {
				return writeUUIDFKAsyncTaskBinding(t, db, fixture, "fk-task-token-uuid",
					func(stored *AsyncTaskBinding) *string { return stored.TokenUUID })
			},
		},
		"async_task_bindings.channel_uuid": {
			responsible:            "relay adaptors populate AsyncTaskBinding.ChannelUUID before model.SaveAsyncTaskBinding",
			populatedByModelWriter: false,
			write: func(t *testing.T, db *gorm.DB, fixture *uuidWriterFixture) *string {
				return writeUUIDFKAsyncTaskBinding(t, db, fixture, "fk-task-channel-uuid",
					func(stored *AsyncTaskBinding) *string { return stored.ChannelUUID })
			},
		},
		"mcp_tools.server_uuid": {
			responsible:            "model.UpsertMCPTools copies the serverUUID argument onto every tool",
			populatedByModelWriter: true,
			wantUUID:               func(fixture *uuidWriterFixture) string { return fixture.server.UUID },
			write: func(t *testing.T, db *gorm.DB, fixture *uuidWriterFixture) *string {
				tool := &MCPTool{Name: "fk-tool"}
				require.NoError(t, UpsertMCPTools(fixture.server.Id, fixture.server.UUID, []*MCPTool{tool}))
				stored := &MCPTool{}
				require.NoError(t, db.First(stored, "id = ?", tool.Id).Error)
				return stored.ServerUUID
			},
		},
		"passkey_credentials.user_uuid": {
			responsible:            "model.CreatePasskeyCredential resolves the owner uuid before DB.Create",
			populatedByModelWriter: true,
			wantUUID:               func(fixture *uuidWriterFixture) string { return fixture.user.UUID },
			write: func(t *testing.T, db *gorm.DB, fixture *uuidWriterFixture) *string {
				cred := &PasskeyCredential{UserId: fixture.user.Id, CredentialName: "fk-passkey",
					CredentialID: []byte("fk-credential-id"), PublicKey: []byte("fk-public-key")}
				require.NoError(t, CreatePasskeyCredential(cred))
				stored := &PasskeyCredential{}
				require.NoError(t, db.First(stored, "id = ?", cred.Id).Error)
				return stored.UserUUID
			},
		},
		"logs.user_uuid": {
			responsible:            "model.FillLogUserUUIDByID, called by RecordLog and every sibling record helper",
			populatedByModelWriter: true,
			wantUUID:               func(fixture *uuidWriterFixture) string { return fixture.user.UUID },
			write: func(t *testing.T, db *gorm.DB, fixture *uuidWriterFixture) *string {
				RecordLog(context.Background(), fixture.user.Id, LogTypeSystem, "fk writer log")
				stored := &Log{}
				require.NoError(t, db.First(stored, "content = ?", "fk writer log").Error)
				return stored.UserUUID
			},
		},
		"logs.channel_uuid": {
			// FINDING: RecordConsumeLog copies no uuid of its own. The relay billing call sites
			// hand it a log that model.SetLogExternalUUIDs has already stamped, so a caller that
			// skips SetLogExternalUUIDs creates a fillable gap the finalizer would flag.
			responsible:            "relay billing call sites stamp the log with model.SetLogExternalUUIDs before model.RecordConsumeLog",
			populatedByModelWriter: false,
			write: func(t *testing.T, db *gorm.DB, fixture *uuidWriterFixture) *string {
				return writeUUIDFKConsumeLog(t, db, fixture, "fk consume log channel",
					func(stored *Log) *string { return stored.ChannelUUID })
			},
		},
		"logs.token_uuid": {
			responsible:            "relay billing call sites stamp the log with model.SetLogExternalUUIDs before model.RecordConsumeLog",
			populatedByModelWriter: false,
			write: func(t *testing.T, db *gorm.DB, fixture *uuidWriterFixture) *string {
				return writeUUIDFKConsumeLog(t, db, fixture, "fk consume log token",
					func(stored *Log) *string { return stored.TokenUUID })
			},
		},
	}
}

// writeUUIDFKTokenTransaction persists one transaction through model.CreateTokenTransaction and
// returns the requested FK UUID column as it was stored.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle owning token_transactions.
//   - fixture: owner rows referenced by the transaction.
//   - transactionID: unique external transaction identifier.
//   - column: projects the FK UUID column under test from the stored row.
//
// Return values:
//   - *string: persisted FK UUID value.
func writeUUIDFKTokenTransaction(t *testing.T, db *gorm.DB, fixture *uuidWriterFixture,
	transactionID string, column func(stored *TokenTransaction) *string) *string {
	t.Helper()
	txn := &TokenTransaction{
		TransactionID: transactionID,
		TokenId:       fixture.token.Id,
		UserId:        fixture.user.Id,
		LogId:         &fixture.logRow.Id,
		Status:        TokenTransactionStatusPending,
		PreQuota:      10,
	}
	require.NoError(t, CreateTokenTransaction(context.Background(), txn))
	stored := &TokenTransaction{}
	require.NoError(t, db.First(stored, "id = ?", txn.Id).Error)
	return column(stored)
}

// writeUUIDFKAsyncTaskBinding persists one binding through model.SaveAsyncTaskBinding and
// returns the requested FK UUID column as it was stored.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle owning async_task_bindings.
//   - fixture: owner rows referenced by the binding.
//   - taskID: unique external task identifier.
//   - column: projects the FK UUID column under test from the stored row.
//
// Return values:
//   - *string: persisted FK UUID value.
func writeUUIDFKAsyncTaskBinding(t *testing.T, db *gorm.DB, fixture *uuidWriterFixture,
	taskID string, column func(stored *AsyncTaskBinding) *string) *string {
	t.Helper()
	binding := &AsyncTaskBinding{
		TaskID:      taskID,
		TaskType:    "video",
		UserID:      fixture.user.Id,
		TokenID:     fixture.token.Id,
		ChannelID:   fixture.channel.Id,
		ChannelType: 1,
	}
	require.NoError(t, SaveAsyncTaskBinding(context.Background(), binding))
	stored := &AsyncTaskBinding{}
	require.NoError(t, db.First(stored, "task_id = ?", taskID).Error)
	return column(stored)
}

// writeUUIDFKConsumeLog persists one consume log through model.RecordConsumeLog without the
// relay's SetLogExternalUUIDs stamp and returns the requested FK UUID column as it was stored.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle owning logs.
//   - fixture: owner rows referenced by the log.
//   - content: unique log content used to read the row back.
//   - column: projects the FK UUID column under test from the stored row.
//
// Return values:
//   - *string: persisted FK UUID value.
func writeUUIDFKConsumeLog(t *testing.T, db *gorm.DB, fixture *uuidWriterFixture,
	content string, column func(stored *Log) *string) *string {
	t.Helper()
	log := &Log{
		UserId:    fixture.user.Id,
		ChannelId: fixture.channel.Id,
		TokenName: fixture.token.Name,
		ModelName: "gpt-4o",
		Quota:     10,
		Content:   content,
	}
	RecordConsumeLog(context.Background(), log)
	stored := &Log{}
	require.NoError(t, db.First(stored, "content = ?", content).Error)
	return column(stored)
}

// newUUIDWriterFixture creates the owner rows every FK writer case copies from, using ORM
// writer paths only so each owner already carries its hook-assigned UUID.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle receiving the rows.
//
// Return values:
//   - *uuidWriterFixture: populated owner rows.
func newUUIDWriterFixture(t *testing.T, db *gorm.DB) *uuidWriterFixture {
	t.Helper()
	ctx := context.Background()

	inviter := &User{Username: "fk-inviter", Password: "password-hash"}
	require.NoError(t, inviter.Insert(ctx, 0))
	user := &User{Username: "fk-owner", Password: "password-hash"}
	require.NoError(t, user.Insert(ctx, 0))

	channel := &Channel{Type: 1, Name: "fk-channel", Models: "gpt-4o", Config: "{}"}
	require.NoError(t, db.Create(channel).Error)

	token := &Token{UserId: user.Id, Key: "fk-owner-token-key", Name: "fk-owner-token"}
	require.NoError(t, token.Insert(ctx))

	server := &MCPServer{Name: "fk-server", BaseURL: "https://example.com/mcp",
		Protocol: MCPProtocolStreamableHTTP, AuthType: MCPAuthTypeNone}
	require.NoError(t, db.Create(server).Error)

	// Mirror model.RecordLog: resolve the owner uuid first, then create. A bare DB.Create here
	// would seed the fixture with the very fillable gap the compatibility gate must not see.
	logRow := &Log{UserId: user.Id, Type: LogTypeSystem, Content: "fk owner log"}
	FillLogUserUUIDByID(ctx, logRow)
	require.NoError(t, db.Create(logRow).Error)

	for _, owner := range []string{inviter.UUID, user.UUID, channel.UUID, token.UUID, server.UUID, logRow.UUID} {
		requireHyphenatedUUID(t, owner)
	}
	return &uuidWriterFixture{inviter: inviter, user: user, channel: channel,
		token: token, server: server, logRow: logRow}
}

// TestUUIDAwareWritersPopulateDenormalizedFKUUIDs covers UUID-A22: for every denormalized FK
// UUID column in uuidFKRegistry, this records which code path is contractually responsible and
// asserts what the writer reachable from package model actually persists today. Columns whose
// resolution lives in an explicit call site outside package model are asserted to stay NULL
// rather than being given a fictional expectation; their responsible path is named in the case.
func TestUUIDAwareWritersPopulateDenormalizedFKUUIDs(t *testing.T) {
	db, _ := newUnifiedTestTopology(t)
	withLogConsumeEnabled(t, true)
	cases := uuidWriterFKCases()

	registryColumns := map[string]bool{}
	for _, target := range uuidFKRegistry() {
		key := target.table + "." + target.uuidColumn
		registryColumns[key] = true
		require.Containsf(t, cases, key, "fk registry column %s has no writer case", key)
	}
	for key := range cases {
		require.Containsf(t, registryColumns, key, "writer case %s is not an fk registry column", key)
	}
	require.Len(t, cases, len(uuidFKRegistry()), "the registry lists 15 denormalized FK UUID columns")

	fixture := newUUIDWriterFixture(t, db)
	for _, target := range uuidFKRegistry() {
		key := target.table + "." + target.uuidColumn
		testCase := cases[key]
		t.Run(key, func(t *testing.T) {
			stored := testCase.write(t, db, fixture)
			if !testCase.populatedByModelWriter {
				require.Nilf(t, stored,
					"%s is populated by %s, not by the model writer; update this case if that changes",
					key, testCase.responsible)
				return
			}
			require.NotNilf(t, stored, "%s must be populated by %s", key, testCase.responsible)
			requireHyphenatedUUID(t, *stored)
			require.Equalf(t, testCase.wantUUID(fixture), *stored,
				"%s must carry the live owner's uuid written by %s", key, testCase.responsible)
		})
	}
}
