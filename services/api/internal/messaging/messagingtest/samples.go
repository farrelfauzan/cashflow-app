package messagingtest

import "github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"

// sampleLink is the link every sample button carries, like the payment link a reminder mints.
const sampleLink = "https://app.example.id/t/tok"

// Samples returns, for every registered template, a valid message with realistic sample
// parameters and the buttons that template carries in production. Contract tests of a provider
// run every template through it; TestSamplesCoverEveryTemplate keeps it complete, so a new
// template fails that test until it has a sample here.
func Samples() map[string]messaging.Message {
	pay := []messaging.Button{{Title: messaging.ButtonPay, Payload: "invoice", URL: sampleLink}}
	change := []messaging.Button{{Title: messaging.ButtonChangePromise, Payload: "promise", URL: sampleLink}}
	newPromise := []messaging.Button{{Title: messaging.ButtonNewPromise, Payload: "promise", URL: sampleLink}}
	return map[string]messaging.Message{
		messaging.TemplateLoginCode: {
			Template: messaging.TemplateLoginCode,
			Params:   map[string]string{"code": "123456"},
		},
		// Buttons as internal/member/invitation.go sends them.
		messaging.TemplateMemberInvite: {
			Template: messaging.TemplateMemberInvite,
			Params: map[string]string{
				"name": "Mbak Sari", "role": "Admin (Kos Melati)", "organization": "Griya Sejahtera", "inviter": "Pak Budi",
			},
			Buttons: []messaging.Button{{Title: "Masuk", Payload: messaging.TemplateMemberInvite, URL: "https://app.example.id/login"}},
		},
		messaging.TemplateInvoiceDueSoon: {Template: messaging.TemplateInvoiceDueSoon, Buttons: pay, Params: map[string]string{
			"name": "Rina Wulandari", "organization": "Griya Sejahtera", "month": "Desember 2026", "amount": "Rp1.620.000",
			"lines": "Sewa Kamar 3 Rp1.500.000, Listrik November Rp120.000", "due_date": "10 Desember 2026",
		}},
		messaging.TemplateInvoiceDueToday: {Template: messaging.TemplateInvoiceDueToday, Buttons: pay, Params: map[string]string{
			"name": "Rina Wulandari", "organization": "Griya Sejahtera", "month": "Desember 2026", "amount": "Rp1.620.000",
			"lines": "Sewa Kamar 3 Rp1.500.000, Listrik November Rp120.000",
		}},
		messaging.TemplateInvoiceOverdue: {Template: messaging.TemplateInvoiceOverdue, Buttons: pay, Params: map[string]string{
			"name": "Rina Wulandari", "organization": "Griya Sejahtera", "month": "Desember 2026", "amount": "Rp620.000",
			"lines": "Sewa Kamar 3 Rp1.500.000, Listrik November Rp120.000, sudah dibayar Rp1.000.000", "days_late": "3",
		}},
		messaging.TemplateInvoiceRollover: {Template: messaging.TemplateInvoiceRollover, Buttons: pay, Params: map[string]string{
			"name": "Rina Wulandari", "organization": "Griya Sejahtera",
			"summary": "Tagihan Desember 2026 Rp1.500.000 + sisa November 2026 Rp200.000", "total": "Rp1.700.000",
		}},
		messaging.TemplatePromiseReminder: {Template: messaging.TemplatePromiseReminder, Buttons: change, Params: map[string]string{
			"name": "Rina Wulandari", "organization": "Griya Sejahtera", "amount": messaging.AllOutstanding, "date": "20 November 2026",
		}},
		messaging.TemplatePromiseBroken: {Template: messaging.TemplatePromiseBroken, Buttons: newPromise, Params: map[string]string{
			"name": "Rina Wulandari", "organization": "Griya Sejahtera", "date": "20 November 2026", "total": "Rp700.000",
		}},
		messaging.TemplatePromiseRecorded: {Template: messaging.TemplatePromiseRecorded, Buttons: change, Params: map[string]string{
			"name": "Rina Wulandari", "organization": "Griya Sejahtera", "amount": "Rp500.000", "date": "20 November 2026",
		}},
	}
}
