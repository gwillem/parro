package api

import (
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractAuthCode(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"with state", "parro://oauth2:443/?code=ABC123&state=xyz", "ABC123"},
		{"without state", "parro://oauth2:443/?code=ABC123", "ABC123"},
		{"cdata wrapper", `<![CDATA[parro://oauth2:443/?code=ABC123]]>`, "ABC123"},
		{"jwt with dots", "parro://oauth2:443/?code=eyJhbGc.eyJzdWI.sig-part", "eyJhbGc.eyJzdWI.sig-part"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractAuthCode(tc.in)
			if err != nil {
				t.Fatalf("extractAuthCode(%q) error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("extractAuthCode(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestSelectAccountAndGetCode_AJAXBody simulates the multi-account Wicket flow
// where the IdP returns the auth code embedded in an AJAX XML body (200 OK).
func TestSelectAccountAndGetCode_AJAXBody(t *testing.T) {
	const wantCode = "ajax-code-abc123"
	var sawPageLoad, sawAJAX bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/idp/wicket/page" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		// Initial page load has a bare query like "5"; AJAX appends
		// "-1.0-accountKeuze-accounts-account-1&_=<ts>".
		if strings.Contains(r.URL.RawQuery, "accountKeuze") {
			sawAJAX = true
			if got := r.Header.Get("Accept"); !strings.Contains(got, "application/xml") {
				t.Errorf("AJAX Accept header = %q, want xml", got)
			}
			w.Header().Set("Content-Type", "text/xml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>` +
				`<ajax-response><redirect><![CDATA[parro://oauth2:443/?code=` + wantCode + `]]></redirect></ajax-response>`))
			return
		}
		sawPageLoad = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html>account picker</html>"))
	}))
	defer srv.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	location := srv.URL + "/idp/wicket/page?5"

	code, err := selectAccountAndGetCode(client, jar, location, func(string, ...any) {})
	if err != nil {
		t.Fatalf("selectAccountAndGetCode: %v", err)
	}
	if code != wantCode {
		t.Errorf("code = %q, want %q", code, wantCode)
	}
	if !sawPageLoad {
		t.Error("page-load request was never made")
	}
	if !sawAJAX {
		t.Error("AJAX request was never made")
	}
}

// TestSelectAccountAndGetCode_302Redirect simulates the variant where the IdP
// answers the AJAX with a 302 carrying the parro:// URL in Location.
func TestSelectAccountAndGetCode_302Redirect(t *testing.T) {
	const wantCode = "redirect-code-xyz789"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "accountKeuze") {
			w.Header().Set("Location", "parro://oauth2:443/?code="+wantCode+"&state=20260519-1200-login")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html>account picker</html>"))
	}))
	defer srv.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	location := srv.URL + "/idp/wicket/page?7"

	code, err := selectAccountAndGetCode(client, jar, location, func(string, ...any) {})
	if err != nil {
		t.Fatalf("selectAccountAndGetCode: %v", err)
	}
	if code != wantCode {
		t.Errorf("code = %q, want %q", code, wantCode)
	}
}

// TestSelectAccountAndGetCode_MissingCode ensures a malformed AJAX response
// surfaces a useful error instead of returning an empty code.
func TestSelectAccountAndGetCode_MissingCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "accountKeuze") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<ajax-response><redirect><![CDATA[https://elsewhere/?foo=bar]]></redirect></ajax-response>`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	location := srv.URL + "/idp/wicket/page?1"

	if _, err := selectAccountAndGetCode(client, jar, location, func(string, ...any) {}); err == nil {
		t.Fatal("expected error for missing auth code, got nil")
	}
}
