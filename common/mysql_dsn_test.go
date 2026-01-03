package common

import (
	"strings"
	"testing"

	gosqlmysql "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

func TestNormalizeMySQLDSNAddsParseTimeAndUTC(t *testing.T) {
	t.Parallel()
	normalized, err := NormalizeMySQLDSN("user:pass@tcp(localhost:3306)/oneapi")
	require.NoError(t, err)

	cfg, err := gosqlmysql.ParseDSN(normalized)
	require.NoError(t, err)
	require.True(t, cfg.ParseTime)
	require.Equal(t, "UTC", cfg.Loc.String())
}

func TestNormalizeMySQLDSNRespectsExistingOptions(t *testing.T) {
	t.Parallel()
	normalized, err := NormalizeMySQLDSN("user:pass@tcp(localhost:3306)/oneapi?parseTime=false&loc=Asia%2FShanghai&charset=utf8mb4")
	require.NoError(t, err)

	cfg, err := gosqlmysql.ParseDSN(normalized)
	require.NoError(t, err)
	require.True(t, cfg.ParseTime)
	require.Equal(t, "Asia/Shanghai", cfg.Loc.String())
	require.True(t, strings.Contains(normalized, "charset=utf8mb4"))
}

func TestNormalizeMySQLDSNHandlesURLFormat(t *testing.T) {
	t.Parallel()
	normalized, err := NormalizeMySQLDSN("mysql://user:pass@127.0.0.1:3306/oneapi?charset=utf8mb4")
	require.NoError(t, err)

	cfg, err := gosqlmysql.ParseDSN(normalized)
	require.NoError(t, err)
	require.Equal(t, "oneapi", cfg.DBName)
	require.True(t, cfg.ParseTime)
	require.Equal(t, "UTC", cfg.Loc.String())
	require.True(t, strings.Contains(normalized, "charset=utf8mb4"))
}
