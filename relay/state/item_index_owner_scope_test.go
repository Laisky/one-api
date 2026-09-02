package state

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestItemIndexIsOwnerScoped pins that one tenant cannot reach into another's
// item index by naming their item id.
//
// ItemEnvelope.UpstreamItemID is populated from a client-supplied "id" on
// POST /v1/conversations/{id}/items, and the index used to be keyed by that id
// alone. Two consequences, both cross-tenant:
//
//   - squat: indexItems writes the upstream entry with SetNX and discards the
//     result, so whoever claimed the id first owned the key and the real owner's
//     write silently no-opped;
//   - delete: every purge path deleted by bare item id with no owner check, so
//     deleting your own item removed a victim's index entry.
func TestItemIndexIsOwnerScoped(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	victim := OwnerScope{UserID: 7, TokenID: 3}
	attacker := OwnerScope{UserID: 8, TokenID: 4}

	store := NewMemoryStore(DefaultLimits())

	victimRecord := sampleResponseWithUpstreamItems(t, victim)
	_, err := store.CreateResponse(ctx, victimRecord, "")
	require.NoError(t, err)

	sharedItemID := victimRecord.OutputItems[0].UpstreamItemID
	require.NotEmpty(t, sharedItemID)

	// The victim can resolve their own item.
	resolved, err := store.GetItem(ctx, victim, sharedItemID)
	require.NoError(t, err)
	require.NotNil(t, resolved)

	// The attacker stores a record whose item claims the SAME upstream id.
	attackerRecord := sampleResponseWithUpstreamItems(t, attacker)
	_, err = store.CreateResponse(ctx, attackerRecord, "")
	require.NoError(t, err)

	// Each side sees only its own entry, and the attacker's write did not take
	// over the victim's key.
	victimItem, err := store.GetItem(ctx, victim, sharedItemID)
	require.NoError(t, err, "the victim's own index entry must survive another tenant claiming the id")
	require.NotNil(t, victimItem)

	attackerItem, err := store.GetItem(ctx, attacker, sharedItemID)
	require.NoError(t, err)
	require.NotNil(t, attackerItem)

	// Deleting the attacker's response must not remove the victim's entry.
	require.NoError(t, store.DeleteResponse(ctx, attacker, attackerRecord.GatewayResponseID))

	_, err = store.GetItem(ctx, attacker, sharedItemID)
	require.ErrorIs(t, err, ErrNotFound, "the attacker's own entry is gone")

	_, err = store.GetItem(ctx, victim, sharedItemID)
	require.NoError(t, err, "another tenant's delete must not purge the victim's index entry")
}
