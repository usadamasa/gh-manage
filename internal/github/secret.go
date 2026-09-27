package github

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"

	"golang.org/x/crypto/nacl/box"
)

// SecretStore is where a repository secret lives.
type SecretStore string

// Secret stores. 値は API のパスの一部 (repos/{owner}/{repo}/<store>/secrets) になる｡
const (
	SecretsActions    SecretStore = "actions"
	SecretsDependabot SecretStore = "dependabot"
)

func secretsPath(owner, repo string, store SecretStore) (string, error) {
	switch store {
	case SecretsActions, SecretsDependabot:
		return "repos/" + owner + "/" + repo + "/" + string(store) + "/secrets", nil
	default:
		return "", fmt.Errorf("unknown secret store %q", store)
	}
}

// PutSecret seals value with the repository's public key and creates or replaces the secret.
// 値は封緘してから送り､エラーにも載せない｡
func (c *Client) PutSecret(ctx context.Context, owner, repo string, store SecretStore, name, value string) error {
	prefix, err := secretsPath(owner, repo, store)
	if err != nil {
		return err
	}
	var key struct {
		KeyID string `json:"key_id"`
		Key   string `json:"key"`
	}
	if err := c.getJSON(ctx, prefix+"/public-key", &key); err != nil {
		return err
	}
	sealed, err := seal(key.Key, value)
	if err != nil {
		return fmt.Errorf("%s/%s: secret %s: %w", owner, repo, name, err)
	}
	return c.send(ctx, http.MethodPut, prefix+"/"+name, map[string]string{"encrypted_value": sealed, "key_id": key.KeyID})
}

// DeleteSecret deletes the secret called name.
func (c *Client) DeleteSecret(ctx context.Context, owner, repo string, store SecretStore, name string) error {
	prefix, err := secretsPath(owner, repo, store)
	if err != nil {
		return err
	}
	return c.send(ctx, http.MethodDelete, prefix+"/"+name, nil)
}

// seal encrypts value for the base64 Curve25519 public key, as the secrets API requires (libsodium sealed box).
func seal(publicKey, value string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil {
		return "", fmt.Errorf("decode public key: %w", err)
	}
	var pub [32]byte
	if len(raw) != len(pub) {
		return "", fmt.Errorf("public key is %d bytes, want %d", len(raw), len(pub))
	}
	copy(pub[:], raw)
	sealed, err := box.SealAnonymous(nil, []byte(value), &pub, rand.Reader)
	if err != nil {
		return "", fmt.Errorf("seal: %w", err)
	}
	return base64.StdEncoding.EncodeToString(sealed), nil
}
