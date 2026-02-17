package google

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"satvos/internal/domain"
)

func TestVerifyIDToken_AudienceMatch(t *testing.T) {
	v := stubVerifier(t, "web-client-id.apps.googleusercontent.com", &tokenInfoResponse{
		Iss:           "https://accounts.google.com",
		Aud:           "web-client-id.apps.googleusercontent.com",
		Sub:           "sub-123",
		Email:         "user@example.com",
		EmailVerified: "true",
		Name:          "Test User",
	})
	claims, err := v.VerifyIDToken(context.Background(), "token")
	require.NoError(t, err)
	assert.Equal(t, "sub-123", claims.Subject)
	assert.True(t, claims.EmailVerified)
}

func TestVerifyIDToken_TrimmedQuotedClientIDMatch(t *testing.T) {
	v := stubVerifier(t, ` "web-client-id.apps.googleusercontent.com" `, &tokenInfoResponse{
		Iss:           "accounts.google.com",
		Aud:           "web-client-id.apps.googleusercontent.com",
		Sub:           "sub-123",
		Email:         "user@example.com",
		EmailVerified: "true",
		Name:          "Test User",
	})
	_, err := v.VerifyIDToken(context.Background(), "token")
	require.NoError(t, err)
}

func TestVerifyIDToken_CommaSeparatedClientIDsMatch(t *testing.T) {
	v := stubVerifier(t, "backend-client.apps.googleusercontent.com, frontend-client.apps.googleusercontent.com", &tokenInfoResponse{
		Iss:           "accounts.google.com",
		Aud:           "frontend-client.apps.googleusercontent.com",
		Sub:           "sub-123",
		Email:         "user@example.com",
		EmailVerified: "true",
		Name:          "Test User",
	})
	_, err := v.VerifyIDToken(context.Background(), "token")
	require.NoError(t, err)
}

func TestVerifyIDToken_AuthorizedPartyMatch(t *testing.T) {
	v := stubVerifier(t, "frontend-client.apps.googleusercontent.com", &tokenInfoResponse{
		Iss:           "accounts.google.com",
		Aud:           "shared-google-client.apps.googleusercontent.com",
		Azp:           "frontend-client.apps.googleusercontent.com",
		Sub:           "sub-123",
		Email:         "user@example.com",
		EmailVerified: "true",
		Name:          "Test User",
	})
	_, err := v.VerifyIDToken(context.Background(), "token")
	require.NoError(t, err)
}

func TestVerifyIDToken_AudienceMismatch(t *testing.T) {
	v := stubVerifier(t, "configured-client.apps.googleusercontent.com", &tokenInfoResponse{
		Iss:           "accounts.google.com",
		Aud:           "unexpected-client.apps.googleusercontent.com",
		Sub:           "sub-123",
		Email:         "user@example.com",
		EmailVerified: "true",
		Name:          "Test User",
	})
	_, err := v.VerifyIDToken(context.Background(), "token")
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrSocialAuthTokenInvalid)
}

func stubVerifier(t *testing.T, clientID string, response *tokenInfoResponse) *Verifier {
	t.Helper()
	bodyBytes, err := json.Marshal(response)
	require.NoError(t, err)

	v := newVerifier(clientID, "https://tokeninfo.test/tokeninfo")
	v.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(string(bodyBytes))),
				Header:     make(http.Header),
			}, nil
		}),
	}
	return v
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
