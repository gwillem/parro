package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	idpBase   = "https://inloggen.parnassys.net"
	clientID  = "wfK7SFH8gojFJpREk5bx"
	userAgent = "parro/2 CFNetwork/3860.300.31 Darwin/25.2.0"
)

// Login performs the full OAuth2+PKCE login flow and returns tokens.
func Login(email, password string, logger *log.Logger) (*TokenResponse, error) {
	logf := func(format string, args ...any) {
		if logger != nil {
			logger.Printf(format, args...)
		}
	}

	// PKCE: generate code_verifier and code_challenge
	verifier, challenge, err := generatePKCE()
	if err != nil {
		return nil, fmt.Errorf("generate PKCE: %w", err)
	}
	logf("generated PKCE verifier+challenge")

	state := fmt.Sprintf("%s-login", time.Now().Format("20060102-1504"))

	// Cookie jar to track session
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Don't follow redirects to parro:// scheme
			if strings.HasPrefix(req.URL.String(), "parro://") {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	// Step 1: GET /oauth2/authorize → 302 to login page
	logf("step 1: GET /oauth2/authorize")
	authorizeURL := fmt.Sprintf(
		"%s/idp/oauth2/authorize?client_id=%s&redirect_uri=%s&response_type=code&scope=openid&oauth2=authorize&state=%s&code_challenge=%s&code_challenge_method=S256",
		idpBase, clientID, url.QueryEscape("parro://oauth2"), state, challenge,
	)

	resp, err := doGet(client, authorizeURL)
	if err != nil {
		return nil, fmt.Errorf("authorize: %w", err)
	}
	resp.Body.Close()

	// The redirect lands on /idp/?auth=<jwt> — follow it to get the login page HTML
	loginPageURL := resp.Request.URL.String()
	resp, err = doGet(client, loginPageURL)
	if err != nil {
		return nil, fmt.Errorf("login page: %w", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read login page: %w", err)
	}

	// Step 2: Extract form action from HTML
	logf("step 2: extracting form action from login page")
	formAction, err := extractFormAction(string(body))
	if err != nil {
		return nil, err
	}

	// Make form action absolute
	formURL := idpBase + "/idp/" + formAction

	// Step 3: POST credentials
	logf("step 3: POST credentials to %s", formURL)
	formData := url.Values{
		"aanmelden":   {"x"},
		"e-mailadres": {email},
		"wachtwoord":  {password},
	}
	req, err := http.NewRequest(http.MethodPost, formURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)

	// Don't follow redirects from login POST — need to inspect the 302
	loginClient := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err = loginClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("login POST: %w", err)
	}
	loginBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	// Successful login returns 302 to /idp/wicket/page?<N>
	if resp.StatusCode != http.StatusFound {
		// Extract Wicket feedback panel error message
		feedbackRe := regexp.MustCompile(`(?s)feedbackPanelERROR[^>]*>.*?<span[^>]*>([^<]+)`)
		if m := feedbackRe.FindStringSubmatch(string(loginBody)); m != nil {
			return nil, fmt.Errorf("login failed: %s", strings.TrimSpace(m[1]))
		}
		return nil, fmt.Errorf("login failed (status %d): check credentials", resp.StatusCode)
	}

	location := resp.Header.Get("Location")

	// The IdP can take two paths after successful login:
	// A) Single-account guardian: direct redirect to parro://oauth2:443/?code=...
	// B) Multi-account guardian: redirect to /wicket/page?<N> for account selection
	var code string
	if strings.HasPrefix(location, "parro://") {
		code, err = extractAuthCode(location)
		if err != nil {
			return nil, fmt.Errorf("login failed: parro:// redirect without code (%w): %s", err, location)
		}
		logf("step 4-5: skipped (single-account guardian, code received in redirect)")
	} else if strings.Contains(location, "wicket/page") {
		code, err = selectAccountAndGetCode(client, jar, location, logf)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("login failed: unexpected redirect to %s", location)
	}

	// Step 6: Exchange code for tokens
	logf("step 6: exchanging auth code for tokens")
	tokenURL := fmt.Sprintf(
		"%s/idp/oauth2/token?client_id=%s&code=%s&grant_type=authorization_code&code_verifier=%s",
		idpBase, clientID, url.QueryEscape(code), verifier,
	)

	req, err = http.NewRequest(http.MethodPost, tokenURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)

	resp, err = client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange: status %d: %s", resp.StatusCode, body)
	}

	var tok TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, fmt.Errorf("decode token: %w", err)
	}
	return &tok, nil
}

// selectAccountAndGetCode performs the Wicket-based account-selection flow
// for guardians with multiple accounts. It loads the account-selection page,
// triggers the AJAX request to select the first account, and extracts the
// auth code from the resulting redirect (Location header or AJAX XML body).
func selectAccountAndGetCode(
	client *http.Client,
	jar http.CookieJar,
	location string,
	logf func(format string, args ...any),
) (string, error) {
	// Follow the redirect to load the account selection page first
	// (Wicket requires the page to be loaded before AJAX calls work).
	pageURL := location
	if !strings.HasPrefix(pageURL, "http") {
		pageURL = idpBase + "/idp/" + strings.TrimPrefix(location, "/idp/")
	}

	resp, err := doGet(client, pageURL)
	if err != nil {
		return "", fmt.Errorf("account page: %w", err)
	}
	resp.Body.Close()

	// Step 4: Select account (first account = index 1)
	logf("step 4: selecting account")
	// Wicket AJAX pattern: page?<N>-1.0-accountKeuze-accounts-account-1
	accountURL := fmt.Sprintf("%s-1.0-accountKeuze-accounts-account-1&_=%d", pageURL, time.Now().UnixMilli())

	req, err := http.NewRequest(http.MethodGet, accountURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/xml, text/xml, */*; q=0.01")
	req.Header.Set("User-Agent", userAgent)

	// Use a no-redirect client so we can read the AJAX XML response
	noRedirectClient := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err = noRedirectClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("account selection: %w", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return "", fmt.Errorf("read account selection: %w", err)
	}

	// Step 5: Extract auth code from response.
	// Could be AJAX XML: <ajax-response><redirect><![CDATA[parro://oauth2:443/?code=...]]>
	// Or a 302 redirect with the code in the Location header.
	logf("step 5: extracting auth code (status %d)", resp.StatusCode)
	var code string
	if resp.StatusCode == http.StatusFound {
		code, err = extractAuthCode(resp.Header.Get("Location"))
	} else {
		code, err = extractAuthCode(string(body))
	}
	if err != nil {
		return "", fmt.Errorf("%w (status %d, body: %s)", err, resp.StatusCode, string(body[:min(len(body), 500)]))
	}
	return code, nil
}

func doGet(client *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	return client.Do(req)
}

func generatePKCE() (verifier, challenge string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(buf)
	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return verifier, challenge, nil
}

var formActionRe = regexp.MustCompile(`<form[^>]+action="([^"]+)"`)

func extractFormAction(html string) (string, error) {
	m := formActionRe.FindStringSubmatch(html)
	if m == nil {
		return "", fmt.Errorf("login form action not found in HTML")
	}
	// Unescape HTML entities
	action := strings.ReplaceAll(m[1], "&amp;", "&")
	// Strip leading "./" if present
	action = strings.TrimPrefix(action, "./")
	return action, nil
}

// Auth codes are JWT-shaped (base64url + dots); stop at the first character
// that can't appear in one — query separator, CDATA close, whitespace, quote, etc.
var codeRe = regexp.MustCompile(`parro://oauth2[^?]*\?code=([^&\]\s"'<#]+)`)

func extractAuthCode(xmlBody string) (string, error) {
	m := codeRe.FindStringSubmatch(xmlBody)
	if m == nil {
		return "", fmt.Errorf("auth code not found in account selection response")
	}
	return m[1], nil
}
