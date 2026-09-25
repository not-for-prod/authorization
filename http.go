package authorization

import (
	"context"
	"net/http"

	authenticationapi "github.com/not-for-prod/authorization/delivery/api/authentication-service"
)

// InputDecoder converts HTTP input to a driver's MethodInput. For callbacks the
// selection comes from stored challenge state. Decoding must have no side effects.
type InputDecoder = authenticationapi.Decoder

// NewHTTPHandler wires the existing v1 routes, validation, and response mapping.
// The host owns its listener and trusted caller-context middleware. No listener
// is started and no provider or storage calls are performed during construction.
func NewHTTPHandler(ctx context.Context, service Authentication, decode InputDecoder) (http.Handler, error) {
	handler, err := authenticationapi.NewImplementation(service, decode)
	if err != nil {
		return nil, err
	}

	return handler.HTTPHandler(ctx)
}
