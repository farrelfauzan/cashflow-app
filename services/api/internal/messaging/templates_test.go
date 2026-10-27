package messaging

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderLoginCode(t *testing.T) {
	text, err := Render(Message{Template: TemplateLoginCode, Params: map[string]string{"code": "123456"}})
	require.NoError(t, err)
	require.Contains(t, text, "*123456*")
	require.Contains(t, text, "Berlaku 5 menit")
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
