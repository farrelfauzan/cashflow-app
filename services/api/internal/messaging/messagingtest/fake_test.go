package messagingtest

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
)

func TestFakeRecordsMessagesInOrder(t *testing.T) {
	f := &Fake{}
	org := uuid.New()

	r1, err := f.SendTemplate(context.Background(), messaging.Message{OrganizationID: org, To: "+628111", Template: "invoice_h3"})
	require.NoError(t, err)
	r2, err := f.SendTemplate(context.Background(), messaging.Message{OrganizationID: org, To: "+628222", Template: "invoice_h0"})
	require.NoError(t, err)

	require.Equal(t, "fake-1", r1.ProviderMessageID)
	require.Equal(t, "fake-2", r2.ProviderMessageID)

	sent := f.Sent()
	require.Len(t, sent, 2)
	require.Equal(t, "invoice_h3", sent[0].Template)
	require.Equal(t, "invoice_h0", sent[1].Template)
}

func TestFakeCanSimulateAnUndeliverableNumber(t *testing.T) {
	f := &Fake{}
	f.FailWith(messaging.ErrUndeliverable)

	_, err := f.SendTemplate(context.Background(), messaging.Message{To: "+628333"})
	require.ErrorIs(t, err, messaging.ErrUndeliverable)
	require.Empty(t, f.Sent(), "a failed send must not be recorded as sent")

	f.FailWith(nil)
	_, err = f.SendTemplate(context.Background(), messaging.Message{To: "+628333"})
	require.NoError(t, err)
}

func TestSentReturnsACopy(t *testing.T) {
	f := &Fake{}
	_, err := f.SendTemplate(context.Background(), messaging.Message{Template: "a"})
	require.NoError(t, err)

	got := f.Sent()
	got[0].Template = "changed"
	require.Equal(t, "a", f.Sent()[0].Template)
}
