package google

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"satvos/internal/domain"
	"satvos/internal/port"
)

const tokenInfoURL = "https://oauth2.googleapis.com/tokeninfo"

type tokenInfoResponse struct {
	Iss           string `json:"iss"`
	Aud           string `json:"aud"`
	Azp           string `json:"azp"`
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified string `json:"email_verified"`
	Name          string `json:"name"`
}

// Verifier validates Google ID tokens via the tokeninfo endpoint.
type Verifier struct {
	clientIDs  map[string]struct{}
	endpoint   string
	httpClient *http.Client
}

// NewVerifier creates a new Google ID token verifier.
func NewVerifier(clientID string) *Verifier {
	return newVerifier(clientID, tokenInfoURL)
}

func newVerifier(clientID, endpoint string) *Verifier {
	return &Verifier{
		clientIDs: parseClientIDs(clientID),
		endpoint:  endpoint,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (v *Verifier) VerifyIDToken(ctx context.Context, idToken string) (*port.SocialAuthClaims, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.endpoint+"?id_token="+idToken, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating tokeninfo request: %w", err)
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, domain.ErrSocialAuthTokenInvalid
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, domain.ErrSocialAuthTokenInvalid
	}

	var info tokenInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, domain.ErrSocialAuthTokenInvalid
	}

	// Validate audience/authorized party against configured client IDs.
	if !v.isAllowedClientID(info.Aud) && !v.isAllowedClientID(info.Azp) {
		log.Printf("google.VerifyIDToken: token rejected due to audience mismatch (aud=%q azp=%q configured=%v)", info.Aud, info.Azp, clientIDList(v.clientIDs))
		return nil, domain.ErrSocialAuthTokenInvalid
	}

	// Validate issuer
	if info.Iss != "accounts.google.com" && info.Iss != "https://accounts.google.com" {
		return nil, domain.ErrSocialAuthTokenInvalid
	}

	return &port.SocialAuthClaims{
		Subject:       info.Sub,
		Email:         info.Email,
		EmailVerified: info.EmailVerified == "true",
		FullName:      info.Name,
	}, nil
}

func (v *Verifier) isAllowedClientID(clientID string) bool {
	if len(v.clientIDs) == 0 {
		return false
	}
	_, ok := v.clientIDs[normalizeClientID(clientID)]
	return ok
}

func parseClientIDs(raw string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, part := range strings.Split(raw, ",") {
		normalized := normalizeClientID(part)
		if normalized == "" {
			continue
		}
		out[normalized] = struct{}{}
	}
	return out
}

func normalizeClientID(v string) string {
	return strings.Trim(strings.TrimSpace(v), `"'`)
}

func clientIDList(ids map[string]struct{}) []string {
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	return out
}

func (v *Verifier) Provider() string {
	return string(domain.AuthProviderGoogle)
}

// Compile-time check.
var _ port.SocialTokenVerifier = (*Verifier)(nil)
