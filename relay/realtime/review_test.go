package realtime

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReviewLedgerGrowthHasAnExplicitStop proves receipt and item-state limits
// are signaled to the caller, not silently evicted or allowed to grow forever.
func TestReviewLedgerGrowthHasAnExplicitStop(t *testing.T) {
	for _,kind:=range []string{"responses","items"}{
		t.Run(kind,func(t *testing.T){
			ledger:=NewLedger()
			var err error
			for i:=0;i<20000;i++{
				frame:=fmt.Sprintf(`{"type":"response.done","response":{"id":"r%d","usage":{"input_tokens":1,"output_tokens":0}}}`,i)
				if kind=="items"{frame=fmt.Sprintf(`{"type":"conversation.item.created","item":{"id":"i%d"}}`,i)}
				err=ledger.Observe([]byte(frame))
				if err!=nil{break}
			}
			require.Error(t,err,"growth must terminate with a surfaced error")
			records,items,seen:=len(ledger.Records),len(ledger.itemModels),len(ledger.seenResponses)
			for i:=0;i<10;i++{require.Error(t,ledger.Observe([]byte(fmt.Sprintf(`{"type":"response.created","response":{"id":"extra%d"}}`,i))))}
			require.Equal(t,records,len(ledger.Records))
			require.Equal(t,items,len(ledger.itemModels))
			require.Equal(t,seen,len(ledger.seenResponses))
			if kind=="responses"{require.NotEmpty(t,ledger.Records,"received billable usage must remain available for settlement")}
		})
	}
}

// TestReviewDuplicateResponsesDoNotConsumeCapacity guards against treating replay
// as new billable work or falsely hitting the resource limit.
func TestReviewDuplicateResponsesDoNotConsumeCapacity(t *testing.T){
	ledger:=NewLedger()
	frame:=[]byte(`{"type":"response.done","response":{"id":"r","usage":{"input_tokens":1,"output_tokens":0}}}`)
	for i:=0;i<20000;i++{require.NoError(t,ledger.Observe(frame))}
	require.Len(t,ledger.Records,1)
	require.False(t,ledger.Incomplete())
}
