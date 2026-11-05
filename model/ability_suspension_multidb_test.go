package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// abilityRow is the persisted routing state of one ability.
type abilityRow struct {
	Enabled      bool
	Priority     int64
	SuspendUntil *time.Time
}

// readAbilityRows returns every ability of a channel keyed by "group/model".
// Parameters: t is the test handle, db is the database, and id identifies the channel.
// Returns: the channel's abilities; a NULL priority reads as -1 so it is never mistaken for 0.
func readAbilityRows(t *testing.T, db *gorm.DB, id int) map[string]abilityRow {
	t.Helper()
	var abilities []Ability
	require.NoError(t, db.Where("channel_id = ?", id).Find(&abilities).Error)
	rows := make(map[string]abilityRow, len(abilities))
	for _, ability := range abilities {
		priority := int64(-1)
		if ability.Priority != nil {
			priority = *ability.Priority
		}
		rows[ability.Group+"/"+ability.Model] = abilityRow{
			Enabled: ability.Enabled, Priority: priority, SuspendUntil: ability.SuspendUntil,
		}
	}
	return rows
}

// requireSuspended asserts that the ability under key is still suspended.
// Parameters: t is the test handle, rows holds the channel's abilities, and key is "group/model".
// Returns: no values.
func requireSuspended(t *testing.T, rows map[string]abilityRow, key string) {
	t.Helper()
	row, ok := rows[key]
	require.True(t, ok, "ability %s must still exist", key)
	require.NotNil(t, row.SuspendUntil, "ability %s must keep its suspension", key)
	require.True(t, row.SuspendUntil.After(time.Now()), "ability %s must still be suspended", key)
}

// requireNotSuspended asserts that the ability under key exists and routes.
// Parameters: t is the test handle, rows holds the channel's abilities, and key is "group/model".
// Returns: no values.
func requireNotSuspended(t *testing.T, rows map[string]abilityRow, key string) {
	t.Helper()
	row, ok := rows[key]
	require.True(t, ok, "ability %s must exist", key)
	require.Nil(t, row.SuspendUntil, "ability %s must not be suspended", key)
}

// TestChannelWritesPreserveAbilitySuspensionMultiDB reproduces the review's P1 on SQLite,
// MySQL (MYSQL_DSN), and PostgreSQL (PG_DSN): a channel write used to delete and recreate
// every ability, so saving SKIP, or any other setting, silently lifted an active
// relay-time suspension and sent traffic back to a channel that had just failed. A write
// must keep the suspension of every (group, model) it keeps, unless it changed the
// upstream the suspension was measured against.
// Parameters: t is the test handle. Returns: no values.
func TestChannelWritesPreserveAbilitySuspensionMultiDB(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := useTestingModelMatrixBackend(t, dialect)
			ctx := context.Background()

			// suspended inserts a two-model channel and suspends its gpt-4o ability.
			suspended := func(t *testing.T, label string) (*Channel, string) {
				t.Helper()
				channel := insertTestingModelMatrixChannel(t, dialect, label, nil)
				require.NoError(t, SuspendAbility(ctx, channel.Group, "gpt-4o", channel.Id, time.Hour))
				requireSuspended(t, readAbilityRows(t, db, channel.Id), channel.Group+"/gpt-4o")
				return channel, channel.Group
			}

			t.Run("saving SKIP keeps the suspension", func(t *testing.T) {
				channel, group := suspended(t, "skip")
				skip := ChannelTestingModelSkip
				require.NoError(t, (&Channel{Id: channel.Id, TestingModel: &skip}).UpdateWithContext(ctx))

				rows := readAbilityRows(t, db, channel.Id)
				requireSuspended(t, rows, group+"/gpt-4o")
				requireNotSuspended(t, rows, group+"/gpt-4o-mini")
				_, err := GetRandomSatisfiedChannel(group, "gpt-4o", false)
				require.Error(t, err, "the suspended ability must still be excluded from routing")
				routed, err := GetRandomSatisfiedChannel(group, "gpt-4o-mini", false)
				require.NoError(t, err)
				require.Equal(t, channel.Id, routed.Id)
			})

			t.Run("an unrelated save keeps the suspension", func(t *testing.T) {
				channel, group := suspended(t, "rename")
				require.NoError(t, (&Channel{Id: channel.Id, Name: channel.Name + "-renamed"}).UpdateWithContext(ctx))
				requireSuspended(t, readAbilityRows(t, db, channel.Id), group+"/gpt-4o")
			})

			t.Run("a model list change keeps retained suspensions", func(t *testing.T) {
				channel, group := suspended(t, "models")
				require.NoError(t, (&Channel{Id: channel.Id, Models: "gpt-4o,o3-mini"}).UpdateWithContext(ctx))

				rows := readAbilityRows(t, db, channel.Id)
				require.Len(t, rows, 2, "the removed model's ability must be dropped")
				requireSuspended(t, rows, group+"/gpt-4o")
				requireNotSuspended(t, rows, group+"/o3-mini")
			})

			t.Run("priority and status reach retained abilities", func(t *testing.T) {
				channel, group := suspended(t, "prio")
				priority := int64(7)
				update := &Channel{Id: channel.Id, Priority: &priority, Status: ChannelStatusManuallyDisabled}
				require.NoError(t, update.UpdateWithContext(ctx))

				rows := readAbilityRows(t, db, channel.Id)
				for key, row := range rows {
					require.False(t, row.Enabled, "ability %s must follow the disabled status", key)
					require.EqualValues(t, 7, row.Priority, "ability %s must follow the new priority", key)
				}
				requireSuspended(t, rows, group+"/gpt-4o")
			})

			t.Run("adding a group keeps the original suspension", func(t *testing.T) {
				channel, group := suspended(t, "group")
				extra := group + "x"
				require.NoError(t, (&Channel{Id: channel.Id, Group: group + "," + extra}).UpdateWithContext(ctx))

				rows := readAbilityRows(t, db, channel.Id)
				require.Len(t, rows, 4)
				requireSuspended(t, rows, group+"/gpt-4o")
				requireNotSuspended(t, rows, extra+"/gpt-4o")
			})

			t.Run("a case-only model rename replaces the ability", func(t *testing.T) {
				channel, group := suspended(t, "case")
				require.NoError(t, (&Channel{Id: channel.Id, Models: "GPT-4O,gpt-4o-mini"}).UpdateWithContext(ctx))

				rows := readAbilityRows(t, db, channel.Id)
				require.Len(t, rows, 2)
				requireNotSuspended(t, rows, group+"/GPT-4O")
				requireNotSuspended(t, rows, group+"/gpt-4o-mini")
			})

			t.Run("rotating the key lifts suspensions", func(t *testing.T) {
				channel, group := suspended(t, "key")
				require.NoError(t, (&Channel{Id: channel.Id, Key: "sk-rotated"}).UpdateWithContext(ctx))
				requireNotSuspended(t, readAbilityRows(t, db, channel.Id), group+"/gpt-4o")
			})

			t.Run("changing the model mapping lifts suspensions", func(t *testing.T) {
				channel, group := suspended(t, "mapping")
				mapping := `{"gpt-4o":"gpt-4.1"}`
				update := &Channel{Id: channel.Id, ModelMapping: &mapping,
					NullableFieldsProvided: map[string]bool{"model_mapping": true}}
				require.NoError(t, update.UpdateWithContext(ctx))
				requireNotSuspended(t, readAbilityRows(t, db, channel.Id), group+"/gpt-4o")
			})

			t.Run("a model reset keeps a retained default's suspension", func(t *testing.T) {
				channel, group := suspended(t, "reset")
				_, err := ResetChannelModelsToDefaults(ctx, channel.Id, func(int) []string {
					return []string{"gpt-4o", "gpt-4.1"}
				})
				require.NoError(t, err)

				rows := readAbilityRows(t, db, channel.Id)
				require.Len(t, rows, 2)
				requireSuspended(t, rows, group+"/gpt-4o")
				requireNotSuspended(t, rows, group+"/gpt-4.1")
			})

			t.Run("UpdateAbilities keeps the suspension", func(t *testing.T) {
				channel, group := suspended(t, "abil")
				var stored Channel
				require.NoError(t, db.First(&stored, "id = ?", channel.Id).Error)
				require.NoError(t, stored.UpdateAbilities())
				requireSuspended(t, readAbilityRows(t, db, channel.Id), group+"/gpt-4o")
			})

			t.Run("a NULL legacy priority is repaired", func(t *testing.T) {
				channel, group := suspended(t, "null")
				require.NoError(t, db.Model(&Ability{}).Where("channel_id = ?", channel.Id).
					Update("priority", nil).Error)
				require.NoError(t, (&Channel{Id: channel.Id, Name: channel.Name + "-x"}).UpdateWithContext(ctx))

				rows := readAbilityRows(t, db, channel.Id)
				for key, row := range rows {
					require.Zero(t, row.Priority, fmt.Sprintf("ability %s must route at the channel priority", key))
				}
				requireSuspended(t, rows, group+"/gpt-4o")
			})
		})
	}
}
