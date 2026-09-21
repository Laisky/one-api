package controller

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/model"
)

// TestTrace395ReadQueryBudget measures real SQL statements for both retained
// trace read routes over repeated calls. Parameters: t is the test handle.
// Returns: none. The UUID compatibility route is the control; the correlation
// route used by Modern must avoid duplicate log reads without omitting ownership.
func TestTrace395ReadQueryBudget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)
	router := traceAccessTestRouter(fixture.user.Id, fixture.user.Id+100)
	queries := 0
	count := func(*gorm.DB) { queries++ }
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register("trace395:count_query", count))
	require.NoError(t, model.DB.Callback().Row().Before("gorm:row").Register("trace395:count_row", count))
	for _, viewer := range []string{"owner", "admin", "root"} {
		t.Run(viewer, func(t *testing.T) {
			wantDirect := 1
			if viewer == "owner" {
				wantDirect = 2 // ownership existence query plus the retained trace
			}
			for sample := 0; sample < 10; sample++ {
				queries = 0
				before := decodeUUIDContractData(t, performTraceAccessRequest(router, "/api/trace/log/"+fixture.log.UUID, viewer))
				legacyCount := queries
				queries = 0
				after := decodeUUIDContractData(t, performTraceAccessRequest(router, "/api/trace/"+fixture.trace.TraceId, viewer))
				require.Equal(t, before["trace_id"], after["trace_id"])
				require.Equal(t, before["timestamps"], after["timestamps"])
				require.Equal(t, before["durations"], after["durations"])
				require.Equal(t, 3, legacyCount, "UUID lookup, narrow log projection and trace lookup")
				require.Equal(t, wantDirect, queries)
				t.Logf("TRACE395_QUERY_BUDGET viewer=%s sample=%d uuid_queries=%d correlation_queries=%d", viewer, sample, legacyCount, queries)
			}
		})
	}
}

// TestTrace395LogReferenceErrorClasses pins the real UUID boundary and prevents
// lookup outages being reported as invalid or missing client references.
// Parameters: t is the test handle. Returns: none; assertions cover public HTTP
// status, safe response text, and cancellation during the projection query.
func TestTrace395LogReferenceErrorClasses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)
	router := traceAccessTestRouter(fixture.user.Id, fixture.user.Id+100)
	for _, tc := range []struct {
		ref    string
		status int
	}{
		{"395", http.StatusBadRequest},
		{"invalid", http.StatusBadRequest},
		{"018f0000-0000-7000-8000-000000000999", http.StatusNotFound},
	} {
		recorder := performTraceAccessRequest(router, "/api/trace/log/"+tc.ref, "root")
		require.Equal(t, tc.status, recorder.Code, recorder.Body.String())
	}

	t.Run("lookup outage", func(t *testing.T) {
		failure := errors.New("private storage endpoint must not leak")
		require.NoError(t, model.DB.Callback().Row().Before("gorm:row").Register("trace395:lookup_failure", func(tx *gorm.DB) {
			tx.AddError(failure)
		}))
		t.Cleanup(func() { require.NoError(t, model.DB.Callback().Row().Remove("trace395:lookup_failure")) })
		recorder := performTraceAccessRequest(router, "/api/trace/log/"+fixture.log.UUID, "root")
		require.Equal(t, http.StatusInternalServerError, recorder.Code)
		require.NotContains(t, recorder.Body.String(), failure.Error())
	})

	t.Run("cancellation during projection", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		projections := 0
		require.NoError(t, model.DB.Callback().Row().Before("gorm:row").Register("trace395:projection_cancel", func(tx *gorm.DB) {
			if strings.Contains(tx.Statement.SQL.String(), "SELECT id, uuid, user_id") {
				projections++
				cancel()
			}
		}))
		t.Cleanup(func() { require.NoError(t, model.DB.Callback().Row().Remove("trace395:projection_cancel")) })
		log, err := model.GetLogForTraceWithContext(ctx, fixture.log.UUID)
		require.Equal(t, 1, projections)
		require.Nil(t, log)
		require.ErrorIs(t, err, context.Canceled)
	})
}
