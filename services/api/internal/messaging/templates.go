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
	TemplateLoginCode = "login_code"
)

// Template is a message the app can send.
//
// Text is a Go text/template used by providers that send plain text (GOWA, devlog). Params lists
// the parameters in order: a provider with registered templates (WhatsApp Cloud API) would send
// them in that order under the same name. WhatsApp formatting applies: *bold*, _italic_.
type Template struct {
	Name   string
	Params []string
	Text   string
	parsed *template.Template
}

var templates = map[string]*Template{}

func register(t Template) {
	t.parsed = template.Must(template.New(t.Name).Option("missingkey=error").Parse(t.Text))
	templates[t.Name] = &t
}

func init() {
	// Platform message, not tied to an organization, so it names none.
	register(Template{
		Name:   TemplateLoginCode,
		Params: []string{"code"},
		Text: "Kode masuk Anda: *{{.code}}*\n\n" +
			"Berlaku 5 menit. Jangan berikan kode ini kepada siapa pun, termasuk orang yang mengaku petugas.",
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

	var b strings.Builder
	if err := t.parsed.Execute(&b, m.Params); err != nil {
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
