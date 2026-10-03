package model

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// TestAsyncTaskPayloadColumnCapacity exercises actual dialect DDL generation.
// MySQL TEXT holds only 64 KiB, less than the gateway's 1 MiB input limit.
func TestAsyncTaskPayloadColumnCapacity(t *testing.T) {
	parsed, err := schema.Parse(&AsyncTask{}, &sync.Map{}, schema.NamingStrategy{})
	require.NoError(t, err)
	dialects := []struct {
		name    string
		dialect gorm.Dialector
		want    string
	}{
		{"mysql", mysql.New(mysql.Config{DSN: "root@tcp(127.0.0.1:1)/unused", SkipInitializeWithVersion: true}), "mediumtext"},
		{"postgres", postgres.New(postgres.Config{DSN: "host=127.0.0.1 port=1 user=unused dbname=unused sslmode=disable"}), "varchar(1048576)"},
	}
	for _, backend := range dialects {
		t.Run(backend.name, func(t *testing.T) {
			db, err := gorm.Open(backend.dialect, &gorm.Config{DisableAutomaticPing: true, DryRun: true})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			for _, name := range []string{"RequestBody", "ResultJSON"} {
				field := parsed.FieldsByName[name]
				require.NotNil(t, field)
				require.Equal(t, backend.want, db.Dialector.DataTypeOf(field))
			}
		})
	}
}
