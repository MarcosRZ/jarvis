// Command ha-dev-setup prepares the development Home Assistant from
// server/compose.yaml for the voice assistant: it creates a user, saves a
// long-lived access token to a file, and gives the demo lights and switches
// Spanish names. It is a development tool, not part of the client.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"jarvis/client/internal/homeassistant"
)

const (
	devUser        = "dev"
	tokenName      = "voice-assistant dev"
	tokenDays      = 3650
	setupTimeout   = 3 * time.Minute
	requestTimeout = 10 * time.Second
	pollInterval   = 2 * time.Second
)

// spanishNames are the names for the entities of the demo integration. Each
// name has its article, so that spoken replies read well.
var spanishNames = map[string]string{
	"light.bed_light":                   "La luz del dormitorio",
	"light.ceiling_lights":              "La luz del techo",
	"light.kitchen_lights":              "La luz de la cocina",
	"light.living_room_rgbww_lights":    "La luz del salón",
	"light.office_rgbw_lights":          "La luz del despacho",
	"light.entrance_color_white_lights": "La luz de la entrada",
	"switch.decorative_lights":          "Las luces decorativas",
	"switch.ac":                         "El aire acondicionado",
}

func main() {
	base := flag.String("url", "http://127.0.0.1:8123", "Home Assistant URL")
	tokenFile := flag.String("token-file", "", "file to write the long-lived access token to (required)")
	passwordFile := flag.String("password-file", "", "file to write the password of the web login to (required)")
	flag.Parse()
	if *tokenFile == "" || *passwordFile == "" {
		fmt.Fprintln(os.Stderr, "--token-file and --password-file are required")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
	defer cancel()
	if err := setup(ctx, strings.TrimRight(*base, "/"), *tokenFile, *passwordFile); err != nil {
		fmt.Fprintln(os.Stderr, "ha-dev-setup:", err)
		os.Exit(1)
	}
}

func setup(ctx context.Context, base, tokenFile, passwordFile string) error {
	client := &http.Client{Timeout: requestTimeout}
	done, err := waitForOnboarding(ctx, client, base)
	if err != nil {
		return err
	}
	if done {
		if tokenWorks(ctx, client, base, tokenFile) {
			fmt.Printf("Home Assistant at %s is already set up; the token in %s works.\n", base, tokenFile)
			return nil
		}
		return fmt.Errorf("Home Assistant at %s is already set up, but %s has no working token; run \"make ha-reset\" to start again, or create a token in Profile > Security and write it to that file", base, tokenFile)
	}

	password, err := randomPassword()
	if err != nil {
		return err
	}
	clientID := base + "/"
	var created struct {
		AuthCode string `json:"auth_code"`
	}
	user := map[string]string{"client_id": clientID, "name": "Dev", "username": devUser, "password": password, "language": "es"}
	if err := postJSON(ctx, client, base+"/api/onboarding/users", "", user, &created); err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	access, err := exchangeCode(ctx, client, base, clientID, created.AuthCode)
	if err != nil {
		return err
	}
	steps := []struct {
		path string
		body any
	}{
		{"/api/onboarding/core_config", map[string]any{}},
		{"/api/onboarding/analytics", map[string]any{}},
		{"/api/onboarding/integration", map[string]string{"client_id": clientID, "redirect_uri": clientID}},
	}
	for _, s := range steps {
		if err := postJSON(ctx, client, base+s.path, access, s.body, nil); err != nil {
			return fmt.Errorf("onboarding %s: %w", s.path, err)
		}
	}

	session, err := homeassistant.Connect(ctx, homeassistant.Config{URL: base, Token: access, Timeout: requestTimeout})
	if err != nil {
		return err
	}
	defer session.Close()
	var token string
	if err := session.Call(ctx, map[string]any{"type": "auth/long_lived_access_token", "client_name": tokenName, "lifespan": tokenDays}, &token); err != nil {
		return err
	}
	for id, name := range spanishNames {
		if err := session.Call(ctx, map[string]any{"type": "config/entity_registry/update", "entity_id": id, "name": name}, nil); err != nil {
			return fmt.Errorf("rename %s: %w", id, err)
		}
	}
	if err := os.WriteFile(tokenFile, []byte(token+"\n"), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(passwordFile, []byte(password+"\n"), 0o600); err != nil {
		return err
	}
	fmt.Printf("Home Assistant at %s is set up.\n  web login: user %s, password in %s\n  token: written to %s\n", base, devUser, passwordFile, tokenFile)
	return nil
}

// waitForOnboarding waits until Home Assistant answers, and reports whether
// its user step is done.
func waitForOnboarding(ctx context.Context, client *http.Client, base string) (bool, error) {
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/onboarding", nil)
		if err != nil {
			return false, err
		}
		resp, err := client.Do(req)
		if err == nil {
			done, err := userStepDone(resp.Body)
			resp.Body.Close()
			if err == nil && resp.StatusCode == http.StatusOK {
				return done, nil
			}
		}
		select {
		case <-time.After(pollInterval):
		case <-ctx.Done():
			return false, fmt.Errorf("Home Assistant at %s did not answer: %w", base, ctx.Err())
		}
	}
}

func userStepDone(body io.Reader) (bool, error) {
	var steps []struct {
		Step string `json:"step"`
		Done bool   `json:"done"`
	}
	if err := json.NewDecoder(body).Decode(&steps); err != nil {
		return false, err
	}
	for _, s := range steps {
		if s.Step == "user" {
			return s.Done, nil
		}
	}
	return false, errors.New("onboarding status has no user step")
}

func tokenWorks(ctx context.Context, client *http.Client, base, tokenFile string) bool {
	b, err := os.ReadFile(tokenFile)
	if err != nil {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(b)))
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func exchangeCode(ctx context.Context, client *http.Client, base, clientID, code string) (string, error) {
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {clientID}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/auth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("get access token: HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return "", err
	}
	return token.AccessToken, nil
}

func postJSON(ctx context.Context, client *http.Client, target, token string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(string(b)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func randomPassword() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
