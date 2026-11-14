package upload

import "context"

// Finding is what Inspect decided, for black-box tests.
type Finding struct {
	Status, ContentType, Reason, Signature string
}

// Inspect runs inspect.
func Inspect(ctx context.Context, objs Objects, sc Scanner, key string, declared int64) (Finding, error) {
	f, err := inspect(ctx, objs, sc, key, declared)
	return Finding{Status: f.status, ContentType: f.contentType, Reason: f.reason, Signature: f.signature}, err
}

// ValidCreate runs validCreate.
var ValidCreate = validCreate

// Actions are the authz actions the package checks.
var Actions = []string{actionCashRecord, actionCashDeposit, actionPaymentCheck, actionCashRead}
