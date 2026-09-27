package github

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/crypto/nacl/box"
)

// TestSeal checks that a sealed value opens with the matching private key, as GitHub does.
func TestSeal(t *testing.T) {
	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := seal(base64.StdEncoding.EncodeToString(pub[:]), "s3cr3t")
	if err != nil {
		t.Fatalf("seal() error = %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		t.Fatalf("sealed value is not base64: %v", err)
	}
	got, ok := box.OpenAnonymous(nil, raw, pub, priv)
	if !ok {
		t.Fatal("OpenAnonymous() failed")
	}
	if string(got) != "s3cr3t" {
		t.Errorf("opened = %q, want s3cr3t", got)
	}
}

func TestSeal_BadKey(t *testing.T) {
	for _, key := range []string{"not base64!", base64.StdEncoding.EncodeToString([]byte("short"))} {
		if _, err := seal(key, "v"); err == nil {
			t.Errorf("seal(%q) error = nil, want error", key)
		}
	}
}

func TestPutSecret(t *testing.T) {
	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyJSON := `{"key_id":"kid-1","key":"` + base64.StdEncoding.EncodeToString(pub[:]) + `"}`
	for _, tt := range []struct {
		store SecretStore
		path  string
	}{
		{SecretsActions, "repos/o/r/actions/secrets"},
		{SecretsDependabot, "repos/o/r/dependabot/secrets"},
	} {
		t.Run(string(tt.store), func(t *testing.T) {
			c, ft := newTestClient(t, map[string]fakeResponse{
				"GET " + tt.path + "/public-key": {body: keyJSON},
				"PUT " + tt.path + "/TOKEN":      {status: http.StatusCreated, body: `{}`},
			})
			if err := c.PutSecret(context.Background(), "o", "r", tt.store, "TOKEN", "s3cr3t"); err != nil {
				t.Fatalf("PutSecret() error = %v", err)
			}
			if len(ft.calls) != 1 || !strings.HasPrefix(ft.calls[0], "PUT "+tt.path+"/TOKEN ") {
				t.Fatalf("calls = %q, want one PUT %s/TOKEN", ft.calls, tt.path)
			}
			if strings.Contains(ft.calls[0], "s3cr3t") {
				t.Fatal("request body carries the plain value")
			}
			var body struct {
				EncryptedValue string `json:"encrypted_value"`
				KeyID          string `json:"key_id"`
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(ft.calls[0], "PUT "+tt.path+"/TOKEN ")), &body); err != nil {
				t.Fatal(err)
			}
			if body.KeyID != "kid-1" {
				t.Errorf("key_id = %q, want kid-1", body.KeyID)
			}
			raw, _ := base64.StdEncoding.DecodeString(body.EncryptedValue)
			if got, ok := box.OpenAnonymous(nil, raw, pub, priv); !ok || string(got) != "s3cr3t" {
				t.Errorf("opened = %q (ok %v), want s3cr3t", got, ok)
			}
		})
	}
}

func TestPutSecret_Errors(t *testing.T) {
	c, _ := newTestClient(t, map[string]fakeResponse{})
	err := c.PutSecret(context.Background(), "o", "r", SecretsActions, "TOKEN", "s3cr3t")
	if err == nil {
		t.Fatal("PutSecret() error = nil, want error")
	}
	if strings.Contains(err.Error(), "s3cr3t") {
		t.Errorf("error %q leaks the value", err)
	}
	if err := c.PutSecret(context.Background(), "o", "r", SecretStore("bogus"), "TOKEN", "v"); err == nil {
		t.Error("PutSecret() with an unknown store: error = nil, want error")
	}
}
