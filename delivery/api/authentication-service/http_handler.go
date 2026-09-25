package authenticationservice

import (
	"context"
	"net/http"

	pb "github.com/not-for-prod/authorization/generated/pb/authorization/v1"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/protobuf/encoding/protojson"
)

// HTTPHandler registers the generated Clay routes. The host wraps this handler
// with its authentication/caller-context middleware, TLS and deployment controls.
// For gRPC, register the same implementation using the generated descriptor.
func (i *Implementation) HTTPHandler(ctx context.Context) (http.Handler, error) {
	mux := runtime.NewServeMux(runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
		MarshalOptions:   protojson.MarshalOptions{EmitUnpopulated: false},
		UnmarshalOptions: protojson.UnmarshalOptions{DiscardUnknown: true},
	}))

	descriptor := pb.NewAuthenticationServiceServiceDesc(i)
	if err := descriptor.RegisterHTTP(ctx, mux); err != nil {
		return nil, err
	}

	const maxRequestBytes = 1 << 20

	bounded := http.MaxBytesHandler(mux, maxRequestBytes)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		bounded.ServeHTTP(w, r)
	}), nil
}
