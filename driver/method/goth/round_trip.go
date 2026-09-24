package goth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

const maxResponseBytes = 1 << 20

type googleTransport struct {
	ctx  context.Context
	base http.RoundTripper
}

func (t *googleTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancel(t.ctx)
	stop := context.AfterFunc(request.Context(), cancel)
	defer stop()
	defer cancel()
	request = request.Clone(ctx)
	// Goth v1.82 puts this token in the query. Use the supported bearer header
	// instead so provider access logs and transport errors cannot expose it in URLs.
	if request.URL.Host == "www.googleapis.com" && request.URL.Path == "/oauth2/v2/userinfo" {
		query := request.URL.Query()
		token := query.Get("access_token")
		query.Del("access_token")
		request.URL.RawQuery = query.Encode()
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := t.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	closeErr := response.Body.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(body) > maxResponseBytes {
		return nil, errors.New("oversized Google response")
	}
	if request.URL.Host == "oauth2.googleapis.com" && response.StatusCode == http.StatusOK {
		// Goth asserts id_token's type. Reject malformed provider data before that
		// assertion; this driver proves the subject through authenticated userinfo.
		var token struct {
			IDToken *string `json:"id_token"`
		}
		if json.Unmarshal(body, &token) != nil {
			return nil, errors.New("malformed Google token response")
		}
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	return response, nil
}
