package messaging

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderLoginCode(t *testing.T) {
	text, err := Render(Message{Template: TemplateLoginCode, Params: map[string]string{"code": "123456"}})
	require.NoError(t, err)
	require.Contains(t, text, "*123456*")
	require.Contains(t, text, "Berlaku 5 menit")
}

func TestRenderMemberInvite(t *testing.T) {
	text, err := Render(Message{
		Template: TemplateMemberInvite,
		Params: map[string]string{
			"name": "Mbak Sari", "role": "Admin (Kos Melati)", "organization": "Griya Sejahtera", "inviter": "Pak Budi",
		},
		Buttons: []Button{{Title: "Masuk", Payload: "member_invite", URL: "https://app.example.id/login"}},
	})
	require.NoError(t, err)
	require.Equal(t, "Halo Mbak Sari,\n\n"+
		"Anda diundang sebagai *Admin (Kos Melati)* di *Griya Sejahtera* oleh Pak Budi.\n\n"+
		"Masuk lewat tautan di bawah dengan nomor WhatsApp ini. Kode masuk akan dikirim ke WhatsApp Anda."+
		"\n\nMasuk:\nhttps://app.example.id/login", text)
}

func TestRenderAppendsButtonLinks(t *testing.T) {
	text, err := Render(Message{
		Template: TemplateLoginCode,
		Params:   map[string]string{"code": "123456"},
		Buttons:  []Button{{Title: "Bayar", Payload: "inv:abc", URL: "https://app.example.id/t/tok"}},
	})
	require.NoError(t, err)
	require.Contains(t, text, "\n\nBayar:\nhttps://app.example.id/t/tok")
}

func TestRenderRejectsMistakes(t *testing.T) {
	tests := map[string]Message{
		"unknown template":      {Template: "nope"},
		"missing parameter":     {Template: TemplateLoginCode},
		"blank parameter":       {Template: TemplateLoginCode, Params: map[string]string{"code": "  "}},
		"unexpected parameter":  {Template: TemplateLoginCode, Params: map[string]string{"code": "1", "kode": "1"}},
		"button without a link": {Template: TemplateLoginCode, Params: map[string]string{"code": "1"}, Buttons: []Button{{Title: "Setujui", Payload: "x"}}},
		"invite missing the organization": {Template: TemplateMemberInvite, Params: map[string]string{
			"name": "Sari", "role": "Admin", "inviter": "Budi",
		}},
		"invite with an extra link parameter": {Template: TemplateMemberInvite, Params: map[string]string{
			"name": "Sari", "role": "Admin", "organization": "Griya", "inviter": "Budi", "link": "https://app.example.id/login",
		}},
	}
	for name, m := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Render(m)
			require.Error(t, err)
		})
	}
}

func TestEveryTemplateRendersWithItsOwnParameters(t *testing.T) {
	for name, tpl := range templates {
		params := map[string]string{}
		for _, p := range tpl.Params {
			params[p] = "x"
		}
		_, err := Render(Message{Template: name, Params: params})
		require.NoError(t, err, name)
	}
}

// tenantMessages are a tenant message of each reminder and promise template with its parameters,
// as the reminder sender fills them.
func tenantMessages() map[string]Message {
	pay := []Button{{Title: ButtonPay, Payload: "invoice", URL: "https://app.example.id/t/tok"}}
	change := []Button{{Title: ButtonChangePromise, Payload: "promise", URL: "https://app.example.id/t/tok"}}
	return map[string]Message{
		TemplateInvoiceDueSoon: {Template: TemplateInvoiceDueSoon, Buttons: pay, Params: map[string]string{
			"name": "Rina Wulandari", "organization": "Griya Sejahtera", "month": "Desember 2026", "amount": "Rp1.620.000",
			"lines": "Sewa Kamar 3 Rp1.500.000, Listrik November Rp120.000", "due_date": "10 Desember 2026",
		}},
		TemplateInvoiceDueToday: {Template: TemplateInvoiceDueToday, Buttons: pay, Params: map[string]string{
			"name": "Rina Wulandari", "organization": "Griya Sejahtera", "month": "Desember 2026", "amount": "Rp1.620.000",
			"lines": "Sewa Kamar 3 Rp1.500.000, Listrik November Rp120.000",
		}},
		TemplateInvoiceOverdue: {Template: TemplateInvoiceOverdue, Buttons: pay, Params: map[string]string{
			"name": "Rina Wulandari", "organization": "Griya Sejahtera", "month": "Desember 2026", "amount": "Rp620.000",
			"lines": "Sewa Kamar 3 Rp1.500.000, Listrik November Rp120.000, sudah dibayar Rp1.000.000", "days_late": "3",
		}},
		TemplateInvoiceRollover: {Template: TemplateInvoiceRollover, Buttons: pay, Params: map[string]string{
			"name": "Rina Wulandari", "organization": "Griya Sejahtera",
			"summary": "Tagihan Desember 2026 Rp1.500.000 + sisa November 2026 Rp200.000", "total": "Rp1.700.000",
		}},
		TemplatePromiseReminder: {Template: TemplatePromiseReminder, Buttons: change, Params: map[string]string{
			"name": "Rina Wulandari", "organization": "Griya Sejahtera", "amount": AllOutstanding, "date": "20 November 2026",
		}},
		TemplatePromiseBroken: {Template: TemplatePromiseBroken, Buttons: []Button{{Title: ButtonNewPromise, Payload: "promise", URL: "https://app.example.id/t/tok"}},
			Params: map[string]string{"name": "Rina Wulandari", "organization": "Griya Sejahtera", "date": "20 November 2026", "total": "Rp700.000"}},
		TemplatePromiseRecorded: {Template: TemplatePromiseRecorded, Buttons: change, Params: map[string]string{
			"name": "Rina Wulandari", "organization": "Griya Sejahtera", "amount": "Rp500.000", "date": "20 November 2026",
		}},
	}
}

func TestRenderTenantMessages(t *testing.T) {
	t.Parallel()
	want := map[string]string{
		TemplateInvoiceDueSoon: "Halo Rina Wulandari,\n\n" +
			"Tagihan sewa *Griya Sejahtera* untuk Desember 2026 sebesar *Rp1.620.000* jatuh tempo pada 10 Desember 2026.\n\n" +
			"Rincian: Sewa Kamar 3 Rp1.500.000, Listrik November Rp120.000\n\n" +
			"Bayar lewat tautan di bawah. Terima kasih.\n\nBayar:\nhttps://app.example.id/t/tok",
		TemplateInvoiceDueToday: "Halo Rina Wulandari,\n\n" +
			"Tagihan sewa *Griya Sejahtera* untuk Desember 2026 sebesar *Rp1.620.000* jatuh tempo hari ini.\n\n" +
			"Rincian: Sewa Kamar 3 Rp1.500.000, Listrik November Rp120.000\n\n" +
			"Bayar lewat tautan di bawah. Jika sudah membayar, abaikan pesan ini. Terima kasih.\n\nBayar:\nhttps://app.example.id/t/tok",
		TemplateInvoiceOverdue: "Halo Rina Wulandari,\n\n" +
			"Tagihan sewa *Griya Sejahtera* untuk Desember 2026 sebesar *Rp620.000* sudah lewat 3 hari dari jatuh tempo.\n\n" +
			"Rincian: Sewa Kamar 3 Rp1.500.000, Listrik November Rp120.000, sudah dibayar Rp1.000.000\n\n" +
			"Bayar lewat tautan di bawah. Jika perlu waktu, Anda bisa membuat janji bayar di halaman yang sama. " +
			"Jika sudah membayar, abaikan pesan ini.\n\nBayar:\nhttps://app.example.id/t/tok",
		TemplateInvoiceRollover: "Halo Rina Wulandari,\n\n" +
			"Tagihan sewa *Griya Sejahtera*:\n" +
			"Tagihan Desember 2026 Rp1.500.000 + sisa November 2026 Rp200.000 = *Rp1.700.000*\n\n" +
			"Bayar lewat satu tautan di bawah; pembayaran melunasi tagihan terlama lebih dulu. " +
			"Jika sudah membayar, abaikan pesan ini.\n\nBayar:\nhttps://app.example.id/t/tok",
		TemplatePromiseReminder: "Halo Rina Wulandari,\n\n" +
			"Mengingatkan janji bayar Anda ke *Griya Sejahtera*: seluruh sisa tagihan pada *20 November 2026*.\n\n" +
			"Bayar atau ubah janji lewat tautan di bawah. Terima kasih.\n\nUbah janji:\nhttps://app.example.id/t/tok",
		TemplatePromiseBroken: "Halo Rina Wulandari,\n\n" +
			"Janji bayar Anda ke *Griya Sejahtera* pada 20 November 2026 belum terpenuhi. Tidak apa-apa, tidak ada denda.\n\n" +
			"Sisa tagihan Anda *Rp700.000*. Silakan bayar, atau buat janji baru lewat tautan di bawah.\n\nBuat janji baru:\nhttps://app.example.id/t/tok",
		TemplatePromiseRecorded: "Halo Rina Wulandari,\n\n" +
			"Janji bayar Anda ke *Griya Sejahtera* sudah tercatat: Rp500.000 pada *20 November 2026*.\n\n" +
			"Untuk membayar atau mengubah janji, buka tautan di bawah. Terima kasih.\n\nUbah janji:\nhttps://app.example.id/t/tok",
	}
	msgs := tenantMessages()
	require.Len(t, msgs, len(want))
	for name, m := range msgs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			text, err := Render(m)
			require.NoError(t, err)
			require.Equal(t, want[name], text)
			require.Contains(t, text, "Griya Sejahtera", "every tenant message names the organization")
		})
	}
}

// TestTenantTemplatesRefuseMistakes: each tenant template refuses a missing parameter, an extra
// one, and a button without a link; no parameter holds a line break (the Cloud API refuses one).
func TestTenantTemplatesRefuseMistakes(t *testing.T) {
	t.Parallel()
	for name, m := range tenantMessages() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tpl, ok := Lookup(name)
			require.True(t, ok)
			require.Equal(t, len(tpl.Params), len(m.Params), "the sample fills every parameter")
			for _, p := range tpl.Params {
				require.NotContains(t, m.Params[p], "\n")
				missing := Message{Template: name, Buttons: m.Buttons, Params: map[string]string{}}
				for k, v := range m.Params {
					if k != p {
						missing.Params[k] = v
					}
				}
				_, err := Render(missing)
				require.ErrorContains(t, err, "needs parameter", "without %s", p)
			}
			extra := Message{Template: name, Buttons: m.Buttons, Params: map[string]string{"link": "https://app.example.id/t/tok"}}
			for k, v := range m.Params {
				extra.Params[k] = v
			}
			_, err := Render(extra)
			require.ErrorContains(t, err, "has no parameter")
			noLink := Message{Template: name, Params: m.Params, Buttons: []Button{{Title: ButtonPay, Payload: "invoice"}}}
			_, err = Render(noLink)
			require.ErrorContains(t, err, "has no link")
		})
	}
}

func TestRenderPlain(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		msg  Message
		want string
	}{
		{
			name: "login code loses its bold markers",
			msg:  Message{Template: TemplateLoginCode, Params: map[string]string{"code": "123456"}},
			want: "Kode masuk Anda: 123456\n\n" +
				"Berlaku 5 menit. Jangan berikan kode ini kepada siapa pun, termasuk orang yang mengaku petugas.",
		},
		{
			name: "an asterisk inside a parameter value survives",
			msg:  Message{Template: TemplateLoginCode, Params: map[string]string{"code": "12*45"}},
			want: "Kode masuk Anda: 12*45\n\n" +
				"Berlaku 5 menit. Jangan berikan kode ini kepada siapa pun, termasuk orang yang mengaku petugas.",
		},
		{
			name: "button links are appended like Render does",
			msg: Message{Template: TemplateLoginCode, Params: map[string]string{"code": "123456"},
				Buttons: []Button{{Title: "Bayar", Payload: "x", URL: "https://app.example.id/t/tok"}}},
			want: "Kode masuk Anda: 123456\n\n" +
				"Berlaku 5 menit. Jangan berikan kode ini kepada siapa pun, termasuk orang yang mengaku petugas." +
				"\n\nBayar:\nhttps://app.example.id/t/tok",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := RenderPlain(tc.msg)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestRenderPlainIsRenderWithoutMarkers(t *testing.T) {
	t.Parallel()
	for name, m := range tenantMessages() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			withMarkers, err := Render(m)
			require.NoError(t, err)
			plain, err := RenderPlain(m)
			require.NoError(t, err)
			require.Equal(t, strings.ReplaceAll(withMarkers, "*", ""), plain, "sample values hold no asterisk")
		})
	}
}

func TestRenderPlainRejectsTheSameMistakes(t *testing.T) {
	t.Parallel()
	for name, m := range map[string]Message{
		"unknown template":      {Template: "nope"},
		"missing parameter":     {Template: TemplateLoginCode},
		"unexpected parameter":  {Template: TemplateLoginCode, Params: map[string]string{"code": "1", "kode": "1"}},
		"button without a link": {Template: TemplateLoginCode, Params: map[string]string{"code": "1"}, Buttons: []Button{{Title: "Setujui", Payload: "x"}}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := RenderPlain(m)
			require.Error(t, err)
		})
	}
}

func TestStripBold(t *testing.T) {
	t.Parallel()
	require.Equal(t, "a {{.x}} b", stripBold("t", "a *{{.x}}* b"))
	require.Equal(t, "{{.due_date}}", stripBold("t", "{{.due_date}}"), "underscores inside an action are parameter names")
	for _, text := range []string{"_miring_", "~coret~", "`kode`", "{{.x}} _miring_"} {
		require.Panics(t, func() { stripBold("t", text) }, text)
	}
	require.Panics(t, func() { stripBold("t", "{{.x") }, "unclosed action")
}

// TestRenderStaffNotices: the staff notices (TG-5) as Telegram shows them (RenderPlain), each
// naming the organization, the tenant and their unit, with no button.
func TestRenderStaffNotices(t *testing.T) {
	t.Parallel()
	who := map[string]string{"organization": "Griya Sejahtera", "tenant": "Rina Wulandari", "unit": "Kamar 3, Kos Melati"}
	with := func(extra map[string]string) map[string]string {
		out := map[string]string{}
		for k, v := range who {
			out[k] = v
		}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}
	tests := []struct {
		name string
		msg  Message
		want string
	}{
		{
			name: "a new promise with an amount",
			msg: Message{Template: TemplateStaffPromiseMade, Params: with(map[string]string{
				"action": StaffPromiseMade, "amount": "Rp500.000", "date": "20 November 2026",
			})},
			want: "Griya Sejahtera: Rina Wulandari (Kamar 3, Kos Melati) membuat janji bayar Rp500.000 " +
				"paling lambat 20 November 2026 lewat halaman pembayaran.",
		},
		{
			name: "an edited promise for everything owed",
			msg: Message{Template: TemplateStaffPromiseMade, Params: with(map[string]string{
				"action": StaffPromiseEdited, "amount": AllOutstanding, "date": "20 November 2026",
			})},
			want: "Griya Sejahtera: Rina Wulandari (Kamar 3, Kos Melati) mengubah janji bayar seluruh sisa tagihan " +
				"paling lambat 20 November 2026 lewat halaman pembayaran.",
		},
		{
			name: "a broken promise",
			msg:  Message{Template: TemplateStaffPromiseBroken, Params: with(map[string]string{"date": "20 November 2026"})},
			want: "Griya Sejahtera: janji bayar Rina Wulandari (Kamar 3, Kos Melati) untuk 20 November 2026 tidak ditepati.",
		},
		{
			name: "a visit task",
			msg:  Message{Template: TemplateStaffVisitTask, Params: with(map[string]string{"reason": "tunggakan lewat 7 hari"})},
			want: "Griya Sejahtera: Rina Wulandari (Kamar 3, Kos Melati) masuk daftar kunjungan karena tunggakan lewat 7 hari.",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			text, err := RenderPlain(tc.msg)
			require.NoError(t, err)
			require.Equal(t, tc.want, text)
		})
	}
}
