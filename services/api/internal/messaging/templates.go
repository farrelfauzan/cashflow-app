package messaging

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"text/template"
)

// Template names. Each one is registered below with its parameters and its Bahasa Indonesia
// text.
const (
	TemplateLoginCode    = "login_code"
	TemplateMemberInvite = "member_invite"

	// Tenant reminders (slice 4, S4-5). Each names the organization (its display name) and the
	// tenant, and carries one button whose link is a freshly minted payment link (decision 39).
	TemplateInvoiceDueSoon  = "invoice_due_soon"  // H-3: one invoice, nothing else owed
	TemplateInvoiceDueToday = "invoice_due_today" // H0: one invoice, nothing else owed
	TemplateInvoiceOverdue  = "invoice_overdue"   // H+3: one invoice, nothing else owed
	TemplateInvoiceRollover = "invoice_rollover"  // any stage while more than one invoice is owed
	TemplatePromiseReminder = "promise_reminder"  // H-1 of a promised date
	TemplatePromiseBroken   = "promise_broken"    // a promise's day passed without it being kept
	TemplatePromiseRecorded = "promise_recorded"  // a promise (or an edit) was saved
)

// Button titles of the tenant messages.
const (
	ButtonPay           = "Bayar"
	ButtonChangePromise = "Ubah janji"
	ButtonNewPromise    = "Buat janji baru"
)

// Template is a message the app can send.
//
// Text is a Go text/template used by providers that send plain text (GOWA, devlog). Params lists
// the parameters in order: a provider with registered templates (WhatsApp Cloud API) would send
// them in that order under the same name. WhatsApp formatting applies, but only *bold*: a
// provider that cannot show it (Telegram) sends RenderPlain, which drops the markers, so Text
// may use no other marker (_italic_, ~strike~, `code`). register refuses a Text that does.
type Template struct {
	Name   string
	Params []string
	Text   string
	parsed *template.Template
	// plain is Text with the *bold* markers removed, parsed for RenderPlain.
	plain *template.Template
}

var templates = map[string]*Template{}

func register(t Template) {
	t.parsed = template.Must(template.New(t.Name).Option("missingkey=error").Parse(t.Text))
	t.plain = template.Must(template.New(t.Name).Option("missingkey=error").Parse(stripBold(t.Name, t.Text)))
	templates[t.Name] = &t
}

// stripBold returns text without WhatsApp's * bold markers. Only the literal text between
// {{ }} actions is touched, so an action is never altered. It panics, as template.Must does at
// register time, when the literal text uses another WhatsApp marker (_, ~ or a backtick), which
// RenderPlain would leave in the message.
func stripBold(name, text string) string {
	var b strings.Builder
	for rest := text; rest != ""; {
		literal, action, found := strings.Cut(rest, "{{")
		if i := strings.IndexAny(literal, "_~`"); i >= 0 {
			panic(fmt.Sprintf("messaging: template %s uses the WhatsApp marker %q, which RenderPlain does not strip", name, literal[i]))
		}
		b.WriteString(strings.ReplaceAll(literal, "*", ""))
		if !found {
			break
		}
		inside, after, closed := strings.Cut(action, "}}")
		if !closed {
			panic(fmt.Sprintf("messaging: template %s has an unclosed action", name))
		}
		b.WriteString("{{" + inside + "}}")
		rest = after
	}
	return b.String()
}

// TemplateNames returns the name of every registered template, sorted.
func TemplateNames() []string {
	return slices.Sorted(maps.Keys(templates))
}

func init() {
	// Platform message, not tied to an organization, so it names none.
	register(Template{
		Name:   TemplateLoginCode,
		Params: []string{"code"},
		Text: "Kode masuk Anda: *{{.code}}*\n\n" +
			"Berlaku 5 menit. Jangan berikan kode ini kepada siapa pun, termasuk orang yang mengaku petugas.",
	})

	// Sent when a member is added to an organization. role is the Indonesian role name, with
	// its property in brackets when limited to one, for example "Admin (Kos Melati)". The
	// login link travels as the message's one button (URL), so text-only providers print it
	// once, under the body; it holds no token.
	register(Template{
		Name:   TemplateMemberInvite,
		Params: []string{"name", "role", "organization", "inviter"},
		Text: "Halo {{.name}},\n\n" +
			"Anda diundang sebagai *{{.role}}* di *{{.organization}}* oleh {{.inviter}}.\n\n" +
			"Masuk lewat tautan di bawah dengan nomor WhatsApp ini. Kode masuk akan dikirim ke WhatsApp Anda.",
	})

	registerTenantTemplates()
}

// registerTenantTemplates adds the tenant reminder and promise messages. Parameters are single
// lines (WhatsApp Cloud API refuses a parameter with a line break), so the line breakdown and the
// rollover sum are one line each: "Sewa Rp1.500.000, Listrik Rp120.000". Amounts are formatted
// with Rupiah and dates with Date or Month before they are passed in.
func registerTenantTemplates() {
	// amount is what the invoice still owes; lines is its breakdown (Rincian), with what was
	// already paid on it when anything was.
	register(Template{
		Name:   TemplateInvoiceDueSoon,
		Params: []string{"name", "organization", "month", "amount", "lines", "due_date"},
		Text: "Halo {{.name}},\n\n" +
			"Tagihan sewa *{{.organization}}* untuk {{.month}} sebesar *{{.amount}}* jatuh tempo pada {{.due_date}}.\n\n" +
			"Rincian: {{.lines}}\n\n" +
			"Bayar lewat tautan di bawah. Terima kasih.",
	})
	register(Template{
		Name:   TemplateInvoiceDueToday,
		Params: []string{"name", "organization", "month", "amount", "lines"},
		Text: "Halo {{.name}},\n\n" +
			"Tagihan sewa *{{.organization}}* untuk {{.month}} sebesar *{{.amount}}* jatuh tempo hari ini.\n\n" +
			"Rincian: {{.lines}}\n\n" +
			"Bayar lewat tautan di bawah. Jika sudah membayar, abaikan pesan ini. Terima kasih.",
	})
	// days_late is a number of days, for example "3".
	register(Template{
		Name:   TemplateInvoiceOverdue,
		Params: []string{"name", "organization", "month", "amount", "lines", "days_late"},
		Text: "Halo {{.name}},\n\n" +
			"Tagihan sewa *{{.organization}}* untuk {{.month}} sebesar *{{.amount}}* sudah lewat {{.days_late}} hari dari jatuh tempo.\n\n" +
			"Rincian: {{.lines}}\n\n" +
			"Bayar lewat tautan di bawah. Jika perlu waktu, Anda bisa membuat janji bayar di halaman yang sama. " +
			"Jika sudah membayar, abaikan pesan ini.",
	})
	// summary is the sum's terms, for example "Tagihan Desember 2026 Rp700.000 + sisa November
	// 2026 Rp200.000"; total is what they add up to, everything the tenant owes.
	register(Template{
		Name:   TemplateInvoiceRollover,
		Params: []string{"name", "organization", "summary", "total"},
		Text: "Halo {{.name}},\n\n" +
			"Tagihan sewa *{{.organization}}*:\n" +
			"{{.summary}} = *{{.total}}*\n\n" +
			"Bayar lewat satu tautan di bawah; pembayaran melunasi tagihan terlama lebih dulu. " +
			"Jika sudah membayar, abaikan pesan ini.",
	})
	// amount is the promised amount (Rupiah) or "seluruh sisa tagihan".
	register(Template{
		Name:   TemplatePromiseReminder,
		Params: []string{"name", "organization", "amount", "date"},
		Text: "Halo {{.name}},\n\n" +
			"Mengingatkan janji bayar Anda ke *{{.organization}}*: {{.amount}} pada *{{.date}}*.\n\n" +
			"Bayar atau ubah janji lewat tautan di bawah. Terima kasih.",
	})
	// date is the promised day that passed; total is everything the tenant still owes.
	register(Template{
		Name:   TemplatePromiseBroken,
		Params: []string{"name", "organization", "date", "total"},
		Text: "Halo {{.name}},\n\n" +
			"Janji bayar Anda ke *{{.organization}}* pada {{.date}} belum terpenuhi. Tidak apa-apa, tidak ada denda.\n\n" +
			"Sisa tagihan Anda *{{.total}}*. Silakan bayar, atau buat janji baru lewat tautan di bawah.",
	})
	register(Template{
		Name:   TemplatePromiseRecorded,
		Params: []string{"name", "organization", "amount", "date"},
		Text: "Halo {{.name}},\n\n" +
			"Janji bayar Anda ke *{{.organization}}* sudah tercatat: {{.amount}} pada *{{.date}}*.\n\n" +
			"Untuk membayar atau mengubah janji, buka tautan di bawah. Terima kasih.",
	})
}

// Lookup returns the registered template with the given name.
func Lookup(name string) (Template, bool) {
	t, ok := templates[name]
	if !ok {
		return Template{}, false
	}
	return *t, true
}

// Render turns a message into the plain text a text-only provider sends: the template filled
// with its parameters, followed by one line per button with its link. It fails on an unknown
// template, a missing or unexpected parameter, or a button without a link, so a mistake shows
// up as an error instead of a broken message in a tenant's chat.
func Render(m Message) (string, error) {
	return render(m, false)
}

// RenderPlain is Render for a provider that cannot show WhatsApp's *bold*, such as Telegram: the
// same checks and the same output, except the markers in the template's own text are gone. A
// parameter value is inserted unchanged, so a * inside one survives.
func RenderPlain(m Message) (string, error) {
	return render(m, true)
}

func render(m Message, plain bool) (string, error) {
	t, ok := templates[m.Template]
	if !ok {
		return "", fmt.Errorf("messaging: unknown template %q", m.Template)
	}
	for _, p := range t.Params {
		if strings.TrimSpace(m.Params[p]) == "" {
			return "", fmt.Errorf("messaging: template %s needs parameter %q", t.Name, p)
		}
	}
	for _, p := range slices.Sorted(maps.Keys(m.Params)) {
		if !slices.Contains(t.Params, p) {
			return "", fmt.Errorf("messaging: template %s has no parameter %q", t.Name, p)
		}
	}

	tpl := t.parsed
	if plain {
		tpl = t.plain
	}
	var b strings.Builder
	if err := tpl.Execute(&b, m.Params); err != nil {
		return "", fmt.Errorf("messaging: rendering template %s: %w", t.Name, err)
	}
	for _, btn := range m.Buttons {
		if btn.URL == "" {
			return "", fmt.Errorf("messaging: button %q has no link, which text-only providers need", btn.Title)
		}
		b.WriteString("\n\n")
		b.WriteString(btn.Title)
		b.WriteString(":\n")
		b.WriteString(btn.URL)
	}
	return b.String(), nil
}
