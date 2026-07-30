package api

import (
	"testing"
)

func TestLoginFormData(t *testing.T) {
	const email = "01AB-v-j.de_Vries"
	pw := "*Xx$ab12Y%!ZZ9Q"
	form := loginFormData(email, pw)

	if got := form.Get("emailadres"); got != email {
		t.Fatalf("emailadres = %q, want %q", got, email)
	}
	if _, ok := form["e-mailadres"]; ok {
		t.Fatal("login form must not submit the obsolete e-mailadres field")
	}
	if got := form.Get("wachtwoord"); got != pw {
		t.Fatalf("wachtwoord = %q, want original password", got)
	}
	if got := form.Get("aanmelden"); got != "x" {
		t.Fatalf("aanmelden = %q, want x", got)
	}
}
