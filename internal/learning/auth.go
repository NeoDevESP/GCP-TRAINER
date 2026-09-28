package learning

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword hashes a password with bcrypt.
func HashPassword(pw string) (string, error) {
	if len(pw) < 8 {
		return "", errors.New("password must have at least 8 characters")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword verifies a bcrypt hash.
func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// Claims is the payload of a session token.
type Claims struct {
	Sub  string `json:"sub"`
	Role string `json:"role"`
	Exp  int64  `json:"exp"`
}

// Tokens issues and verifies HMAC-signed session tokens (JWT-compatible HS256).
type Tokens struct {
	Secret []byte
	TTL    time.Duration
}

var b64 = base64.RawURLEncoding

// Issue creates a token for a user.
func (t *Tokens) Issue(u User) string {
	h := b64.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	c, _ := json.Marshal(Claims{Sub: u.ID, Role: u.Role, Exp: time.Now().Add(t.TTL).Unix()})
	p := b64.EncodeToString(c)
	m := hmac.New(sha256.New, t.Secret)
	m.Write([]byte(h + "." + p))
	return h + "." + p + "." + b64.EncodeToString(m.Sum(nil))
}

// Verify checks signature and expiry.
func (t *Tokens) Verify(tok string) (*Claims, error) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed token")
	}
	m := hmac.New(sha256.New, t.Secret)
	m.Write([]byte(parts[0] + "." + parts[1]))
	sig, err := b64.DecodeString(parts[2])
	if err != nil || !hmac.Equal(sig, m.Sum(nil)) {
		return nil, errors.New("invalid signature")
	}
	raw, err := b64.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var c Claims
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	if time.Now().Unix() > c.Exp {
		return nil, errors.New("token expired")
	}
	return &c, nil
}

// NewID returns a random identifier.
func NewID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

// OIDC implements the authorization-code flow against any OpenID Connect
// provider (Google, Keycloak, Entra ID...). Identity is taken from the
// userinfo endpoint using the access token, so no JWKS handling is needed.
type OIDC struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	disc         *oidcDiscovery
	HTTP         *http.Client
}

type oidcDiscovery struct {
	Authorization string `json:"authorization_endpoint"`
	Token         string `json:"token_endpoint"`
	UserInfo      string `json:"userinfo_endpoint"`
}

func (o *OIDC) discover() (*oidcDiscovery, error) {
	if o.disc != nil {
		return o.disc, nil
	}
	c := o.HTTP
	if c == nil {
		c = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := c.Get(strings.TrimSuffix(o.Issuer, "/") + "/.well-known/openid-configuration")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var d oidcDiscovery
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, err
	}
	o.disc = &d
	return &d, nil
}

// AuthURL returns the provider login URL.
func (o *OIDC) AuthURL(state string) (string, error) {
	d, err := o.discover()
	if err != nil {
		return "", err
	}
	v := url.Values{"client_id": {o.ClientID}, "redirect_uri": {o.RedirectURL}, "response_type": {"code"}, "scope": {"openid email profile"}, "state": {state}}
	return d.Authorization + "?" + v.Encode(), nil
}

// Exchange trades an authorization code for the user's email and name.
func (o *OIDC) Exchange(code string) (email, name string, err error) {
	d, err := o.discover()
	if err != nil {
		return "", "", err
	}
	c := o.HTTP
	if c == nil {
		c = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := c.PostForm(d.Token, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {o.RedirectURL}, "client_id": {o.ClientID}, "client_secret": {o.ClientSecret}})
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil || tok.AccessToken == "" {
		return "", "", fmt.Errorf("token exchange failed")
	}
	req, _ := http.NewRequest("GET", d.UserInfo, nil)
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	r2, err := c.Do(req)
	if err != nil {
		return "", "", err
	}
	defer r2.Body.Close()
	body, _ := io.ReadAll(r2.Body)
	var ui struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Verified bool   `json:"email_verified"`
	}
	if err := json.Unmarshal(body, &ui); err != nil || ui.Email == "" {
		return "", "", fmt.Errorf("userinfo failed")
	}
	return ui.Email, ui.Name, nil
}
