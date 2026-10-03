package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSettlePaidPaymentOrderAcceptsLatePaidEvent(t *testing.T) {
	for _, terminalStatus := range []string{PaymentStatusFailed, PaymentStatusCanceled} {
		t.Run(terminalStatus, func(t *testing.T) {
			setupPaymentTestDB(t)
			userID := 100
			require.NoError(t, DB.Create(&User{
				Id: userID, Username: "late-" + terminalStatus, Password: "x",
				DisplayName: "late", Role: 1, Status: 1, Quota: 10,
			}).Error)
			sessionID := "cs_late_" + terminalStatus
			require.NoError(t, DB.Create(&PaymentOrder{
				UserId: userID, Provider: PaymentProviderStripe, RequestID: "req-" + terminalStatus,
				SessionID: sessionID, AmountCents: 500, Currency: "usd", Quota: 50,
				Status: terminalStatus,
			}).Error)

			transitioned, order, err := SettlePaidPaymentOrder(
				context.Background(), sessionID, time.Now().UnixMilli(), 500, "usd",
			)
			require.NoError(t, err)
			require.True(t, transitioned)
			require.NotNil(t, order)
			require.Equal(t, PaymentStatusPaid, order.Status)

			var user User
			require.NoError(t, DB.First(&user, userID).Error)
			require.Equal(t, int64(60), user.Quota)

			transitioned, _, err = SettlePaidPaymentOrder(
				context.Background(), sessionID, time.Now().UnixMilli(), 500, "usd",
			)
			require.NoError(t, err)
			require.False(t, transitioned)
			require.NoError(t, DB.First(&user, userID).Error)
			require.Equal(t, int64(60), user.Quota)
		})
	}
}
