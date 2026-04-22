package security

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateAPIKey(t *testing.T) {
	key, err := GenerateAPIKey()
	require.NoError(t, err)
	assert.NotEmpty(t, key)
	assert.GreaterOrEqual(t, len(key), 40)
}

func TestExtractAPIKey(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/lives", nil)
	req.Header.Set("Authorization", "Bearer secret")
	assert.Equal(t, "secret", ExtractAPIKey(req))

	req.Header.Set("X-API-Key", " header-secret ")
	assert.Equal(t, "header-secret", ExtractAPIKey(req))
}

func TestSignURLAndValidateSignedRequest(t *testing.T) {
	now := time.Unix(100, 0)
	signedURL, expires, err := SignURL("/files/主播/视频 1.flv", http.MethodGet, now.Add(time.Hour), "secret")
	require.NoError(t, err)
	assert.Equal(t, now.Add(time.Hour).Unix(), expires)

	req := httptest.NewRequest(http.MethodGet, signedURL, nil)
	assert.NoError(t, ValidateSignedRequest(req, "secret", now))

	headReq := httptest.NewRequest(http.MethodHead, signedURL, nil)
	assert.NoError(t, ValidateSignedRequest(headReq, "secret", now))
}

func TestValidateSignedRequestRejectsExpiredOrTamperedURL(t *testing.T) {
	now := time.Unix(100, 0)
	signedURL, _, err := SignURL("/files/video.flv", http.MethodGet, now.Add(time.Second), "secret")
	require.NoError(t, err)

	expiredReq := httptest.NewRequest(http.MethodGet, signedURL, nil)
	assert.ErrorIs(t, ValidateSignedRequest(expiredReq, "secret", now.Add(2*time.Second)), ErrExpiredSignature)

	tamperedReq := httptest.NewRequest(http.MethodGet, signedURL, nil)
	tamperedReq.URL.Path = "/files/other.flv"
	err = ValidateSignedRequest(tamperedReq, "secret", now)
	assert.True(t, errors.Is(err, ErrInvalidSignature), "got %v", err)
}
