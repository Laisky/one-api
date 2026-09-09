package controller

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
	"github.com/Laisky/one-api/relay/realtime"
)

// TestReviewMissingFinalReceiptIsNotIdle exercises the real settlement boundary
// with disconnects, malformed receipts, true idle, and authoritative zero usage.
func TestReviewMissingFinalReceiptIsNotIdle(t *testing.T) {
	for _,tc:=range []struct{name string;frames []string;want int64;estimated bool}{
		{"idle",nil,0,false},
		{"authoritative_zero",[]string{`{"type":"response.done","response":{"id":"r","usage":{"input_tokens":0,"output_tokens":0}}}`},0,false},
		{"disconnect_after_created",[]string{`{"type":"response.created","response":{"id":"r"}}`},1234,true},
		{"missing_usage",[]string{`{"type":"response.done","response":{"id":"r","usage":null}}`},1234,true},
		{"invalid_usage",[]string{`{"type":"response.done","response":{"id":"r","usage":{"input_tokens":-1,"output_tokens":0}}}`},1234,true},
		{"partial_then_disconnect",[]string{`{"type":"response.done","response":{"id":"r1","usage":{"input_tokens":10,"output_tokens":0}}}`,`{"type":"response.created","response":{"id":"r2"}}`},1234,true},
	} {
		t.Run(tc.name,func(t *testing.T){
			ledger:=realtime.NewLedger()
			for _,frame:=range tc.frames{_ = ledger.Observe([]byte(frame))}
			ledger.Finish()
			a:=&openai.Adaptor{}
			got,metadata:=prepareRealtimeReceiptSettlement(quota.ComputeInput{ModelName:"gpt-realtime",ModelRatio:2,GroupRatio:1,PricingAdaptor:a,Usage:&relaymodel.Usage{Realtime:ledger}},1234)
			require.Equal(t,tc.want,got.TotalQuota)
			estimated,_:=metadata[model.LogMetadataKeyEstimatedCharge].(bool)
			require.Equal(t,tc.estimated,estimated)
			if tc.estimated{require.Equal(t,true,metadata["realtime_billing_incomplete"])}
		})
	}
}

// TestReviewReceiptMetadataIsBounded serializes what settlement actually stores,
// rather than measuring the ledger alone. MySQL TEXT allows at most 65535 bytes.
func TestReviewReceiptMetadataIsBounded(t *testing.T) {
	ledger:=realtime.NewLedger()
	for i:=0;i<1000;i++{
		frame:=fmt.Sprintf(`{"type":"response.done","response":{"id":"%s%d","usage":{"input_tokens":1,"output_tokens":0}}}`,strings.Repeat("r",120),i)
		require.NoError(t,ledger.Observe([]byte(frame)))
	}
	ledger.Finish()
	got,metadata:=prepareRealtimeReceiptSettlement(quota.ComputeInput{ModelName:"gpt-realtime",ModelRatio:2,GroupRatio:1,PricingAdaptor:&openai.Adaptor{},Usage:&relaymodel.Usage{Realtime:ledger}},0)
	require.Equal(t,int64(2000),got.TotalQuota,"metadata bounding must not discard billable receipts")
	encoded,err:=json.Marshal(metadata)
	require.NoError(t,err)
	require.Less(t,len(encoded),32*1024,"leave room for other log metadata within TEXT")
}
