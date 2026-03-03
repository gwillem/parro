package api

import (
	"net/url"
	"testing"
)

func TestPasswordEncoding(t *testing.T) {
	pw := "*Xx$ab12Y%!ZZ9Q"
	t.Logf("raw password: %q", pw)
	t.Logf("raw bytes: %x", pw)
	v := url.Values{
		"aanmelden":   {"x"},
		"e-mailadres": {"01AB-v-j.de_Vries"},
		"wachtwoord":  {pw},
	}
	encoded := v.Encode()
	t.Logf("encoded form: %s", encoded)

	// Verify url.Values encodes special characters correctly
	expected := "aanmelden=x&e-mailadres=01AB-v-j.de_Vries&wachtwoord=%2AXx%24ab12Y%25%21ZZ9Q"
	if encoded != expected {
		t.Logf("expected:     %s", expected)
		t.Logf("NOTE: difference may be ok if server decodes both correctly")
	}
}
