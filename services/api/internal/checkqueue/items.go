package checkqueue

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

// loadItems reads the details of a page of waiting items, one query per kind, and keeps the
// page's order.
func loadItems(ctx context.Context, q *dbgen.Queries, org uuid.UUID, rows []dbgen.ListCheckQueueRow) ([]Item, error) {
	ids := map[string][]uuid.UUID{}
	for _, r := range rows {
		ids[r.Kind] = append(ids[r.Kind], r.ItemID)
	}
	d := details{tenants: map[uuid.UUID]*TenantRef{}}
	if err := d.load(ctx, q, org, ids); err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(rows))
	for _, r := range rows {
		it, ok := d.item(r)
		if !ok {
			// The view listed it in the same snapshot, so its source row is there.
			return nil, fmt.Errorf("checkqueue: no details for %s %s", r.Kind, r.ItemID)
		}
		out = append(out, it)
	}
	return out, nil
}

// details holds what loadItems read, by kind and id.
type details struct {
	proofs     map[uuid.UUID]*TransferProof
	gaps       map[uuid.UUID]*DepositGap
	infected   map[uuid.UUID]*UploadInfected
	mismatches map[uuid.UUID]*QrisAmountMismatch
	phones     map[uuid.UUID]*PhoneChanged
	// tenants are shared by the items that name them, so their units are filled in once.
	tenants map[uuid.UUID]*TenantRef
}

func (d *details) tenant(id uuid.UUID, name string) *TenantRef {
	if t, ok := d.tenants[id]; ok {
		return t
	}
	t := &TenantRef{ID: id, Name: name, Units: []Unit{}}
	d.tenants[id] = t
	return t
}

func (d *details) load(ctx context.Context, q *dbgen.Queries, org uuid.UUID, ids map[string][]uuid.UUID) error {
	d.proofs, d.gaps, d.infected = map[uuid.UUID]*TransferProof{}, map[uuid.UUID]*DepositGap{}, map[uuid.UUID]*UploadInfected{}
	d.mismatches, d.phones = map[uuid.UUID]*QrisAmountMismatch{}, map[uuid.UUID]*PhoneChanged{}
	proofTenants := map[uuid.UUID]*TenantRef{}
	if len(ids[KindTransferProof]) > 0 {
		rows, err := q.ListQueueTransferProofs(ctx, dbgen.ListQueueTransferProofsParams{OrganizationID: org, Ids: ids[KindTransferProof]})
		if err != nil {
			return fmt.Errorf("checkqueue: loading proofs: %w", err)
		}
		for _, r := range rows {
			proofTenants[r.ID] = d.tenant(r.TenantID, r.TenantName)
			proof := &TransferProof{InvoiceID: r.InvoiceID.UUID, UploadID: r.UploadID, UploadStatus: r.UploadStatus}
			if r.SentByStaffUserID.Valid && r.SentByName != nil {
				proof.SentBy = &StaffRef{ID: r.SentByStaffUserID.UUID, Name: *r.SentByName}
			}
			d.proofs[r.ID] = proof
		}
	}
	if len(ids[KindDepositGap]) > 0 {
		rows, err := q.ListQueueDepositGaps(ctx, dbgen.ListQueueDepositGapsParams{OrganizationID: org, Ids: ids[KindDepositGap]})
		if err != nil {
			return fmt.Errorf("checkqueue: loading deposit gaps: %w", err)
		}
		for _, r := range rows {
			d.gaps[r.ID] = &DepositGap{
				Depositor: StaffRef{ID: r.StaffUserID, Name: r.StaffName}, Expected: r.ExpectedAmount, Received: r.ReceivedAmount,
				Gap: r.ReceivedAmount - r.ExpectedAmount, PaymentCount: int(r.PaymentCount),
				ConfirmedBy: StaffRef{ID: r.ConfirmedByStaffUserID, Name: r.ConfirmedByName}, ConfirmedAt: r.ConfirmedAt,
			}
		}
	}
	if len(ids[KindUploadInfected]) > 0 {
		rows, err := q.ListQueueInfectedUploads(ctx, dbgen.ListQueueInfectedUploadsParams{OrganizationID: org, Ids: ids[KindUploadInfected]})
		if err != nil {
			return fmt.Errorf("checkqueue: loading infected uploads: %w", err)
		}
		for _, r := range rows {
			it := &UploadInfected{UploadID: r.ID, Purpose: r.Purpose, ScannedAt: r.ScannedAt}
			if r.TenantID.Valid && r.TenantName != nil {
				it.Tenant = d.tenant(r.TenantID.UUID, *r.TenantName)
			}
			if r.UploadedByStaffUserID.Valid && r.UploadedByName != nil {
				it.UploadedBy = &StaffRef{ID: r.UploadedByStaffUserID.UUID, Name: *r.UploadedByName}
			}
			d.infected[r.ID] = it
		}
	}
	mismatchTenants := map[uuid.UUID]*TenantRef{}
	if len(ids[KindQrisAmountMismatch]) > 0 {
		rows, err := q.ListQueueQrisMismatches(ctx, dbgen.ListQueueQrisMismatchesParams{OrganizationID: org, Ids: ids[KindQrisAmountMismatch]})
		if err != nil {
			return fmt.Errorf("checkqueue: loading QRIS mismatches: %w", err)
		}
		for _, r := range rows {
			mismatchTenants[r.ID] = d.tenant(r.TenantID, r.TenantName)
			it := &QrisAmountMismatch{AttemptID: r.ID, TransactionID: r.TransactionID, QRAmount: r.Amount, RecordedAmount: r.PaidAmount}
			if r.AmountMismatch {
				it.ReportedAmount = r.PaidAmount
			}
			d.mismatches[r.ID] = it
		}
	}
	phoneTenants := map[uuid.UUID]*TenantRef{}
	if len(ids[KindPhoneChanged]) > 0 {
		rows, err := q.ListQueuePhoneChanges(ctx, dbgen.ListQueuePhoneChangesParams{OrganizationID: org, Ids: ids[KindPhoneChanged]})
		if err != nil {
			return fmt.Errorf("checkqueue: loading new numbers: %w", err)
		}
		for _, r := range rows {
			phoneTenants[r.ID] = d.tenant(r.TenantID, r.TenantName)
			it := &PhoneChanged{CurrentPhone: r.CurrentPhone, RecordedBy: StaffRef{ID: r.StaffUserID, Name: r.StaffName}}
			if r.NewPhone != nil {
				it.NewPhone = *r.NewPhone
			}
			if r.Note != nil {
				it.Note = *r.Note
			}
			d.phones[r.ID] = it
		}
	}
	if err := d.loadUnits(ctx, q, org); err != nil {
		return err
	}
	// The tenants' units are filled in now; copy them into the items that hold a tenant by value.
	for id, t := range proofTenants {
		d.proofs[id].Tenant = *t
	}
	for id, t := range mismatchTenants {
		d.mismatches[id].Tenant = *t
	}
	for id, t := range phoneTenants {
		d.phones[id].Tenant = *t
	}
	return nil
}

func (d *details) loadUnits(ctx context.Context, q *dbgen.Queries, org uuid.UUID) error {
	if len(d.tenants) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(d.tenants))
	for id := range d.tenants {
		ids = append(ids, id)
	}
	rows, err := q.ListQueueTenantUnits(ctx, dbgen.ListQueueTenantUnitsParams{OrganizationID: org, TenantIds: ids})
	if err != nil {
		return fmt.Errorf("checkqueue: loading the tenants' units: %w", err)
	}
	for _, r := range rows {
		t := d.tenants[r.TenantID]
		t.Units = append(t.Units, Unit{Label: r.UnitLabel, PropertyName: r.PropertyName, Active: r.Active})
	}
	return nil
}

// item builds the queue item of a listed row from what load read.
func (d *details) item(r dbgen.ListCheckQueueRow) (Item, bool) {
	it := Item{Kind: r.Kind, ID: r.ItemID, CreatedAt: r.CreatedAt}
	var ok bool
	switch r.Kind {
	case KindTransferProof:
		it.TransferProof, ok = d.proofs[r.ItemID]
	case KindDepositGap:
		it.DepositGap, ok = d.gaps[r.ItemID]
	case KindUploadInfected:
		it.UploadInfected, ok = d.infected[r.ItemID]
	case KindQrisAmountMismatch:
		it.QrisAmountMismatch, ok = d.mismatches[r.ItemID]
	case KindPhoneChanged:
		it.PhoneChanged, ok = d.phones[r.ItemID]
	}
	return it, ok
}
