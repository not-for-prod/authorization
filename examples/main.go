// A local test server with bcrypt password verification and in-memory state. Run: go run ./examples
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"authorization/domain/entity"
	eventinmem "authorization/driver/event/inmem"
	"authorization/driver/method/logpass"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	clayserver "github.com/not-for-prod/clay/server"
	"github.com/not-for-prod/proterror/proterror"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	authenticationapi "authorization/delivery/api/authentication-service"
	pb "authorization/generated/pb/authorization/v1"

	"authorization"
	identityinmem "authorization/driver/identity/inmem"
	sessioninmem "authorization/driver/session/inmem"
	transactioninmem "authorization/driver/transaction/inmem"
	managerinmem "authorization/driver/transactionmanager/inmem"
	userinmem "authorization/driver/user/inmem"
)

type callerContextKey struct{}

//go:embed web/*
var webFiles embed.FS

const tenant = "main"

func main() {
	manager := managerinmem.New()

	sessions, err := sessioninmem.NewSessionManager(tenant, time.Hour)
	if err != nil {
		log.Fatal(err)
	}

	identityRepo := identityinmem.New(manager)

	deps := authorization.Dependencies{
		Events:             eventinmem.New(manager),
		Transactions:       transactioninmem.New(manager),
		TransactionManager: manager,
		Users:              userinmem.New(manager),
		Identities:         identityRepo,
		Access: func(ctx context.Context, requestedTenant authorization.TenantID) (authorization.Caller, error) {
			caller, ok := ctx.Value(callerContextKey{}).(authorization.Caller)
			if !ok || requestedTenant != tenant {
				return authorization.Caller{}, &proterror.PermissionDenied{}
			}

			return caller, nil
		},
		Eligible: func(_ context.Context, requestedTenant authorization.TenantID, user authorization.User) error {
			if requestedTenant != tenant || user.Status != "active" {
				return &proterror.PermissionDenied{}
			}

			return nil
		},
	}

	logPass, err := logpass.New(identityRepo)
	if err != nil {
		log.Fatal(err)
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal(err)
	}

	err = manager.Do(context.Background(), func(ctx context.Context) error {
		now := time.Now().UTC()
		if err := deps.Users.Create(ctx, authorization.User{
			ID: "alice", Status: entity.UserStatusActive, CreatedAt: now,
		}); err != nil {
			return err
		}

		return identityRepo.Create(ctx, authorization.Identity{
			ID: "alice-local", UserID: "alice", Kind: "local", Issuer: "local",
			Subject: "alice", PasswordHash: string(passwordHash), CreatedAt: now,
		})
	})
	if err != nil {
		log.Fatal(err)
	}

	policy := authorization.Any("primary", authorization.Method(logPass))

	auth, err := authorization.New(
		deps,
		authorization.WithLimits(
			authorization.TransactionSettings{
				Lifetime: 5 * time.Minute, OperationTimeout: 10 * time.Second, MaxAttempts: 10,
				MaxChallenges: 4, MaxChallengeBytes: 4096, MaxPolicyNodes: 128,
			},
		),
		authorization.WithTenant(
			tenant,
			authorization.WithSessionManager(sessions),
			authorization.WithIntent(
				entity.IntentSignIn,
				authorization.WithPolicy(policy),
				authorization.WithSessionIssuance(),
			),
		),
	)
	if err != nil {
		log.Fatal(err)
	}

	handler, err := authenticationapi.NewImplementation(auth, decodeInput)
	if err != nil {
		log.Fatal(err)
	}

	descriptor := pb.NewAuthenticationServiceServiceDesc(handler)

	var callers sync.Map

	port := 8080
	if configured := os.Getenv("AUTH_EXAMPLE_PORT"); configured != "" {
		port, err = strconv.Atoi(configured)
		if err != nil || port < 1 || port > 65535 {
			log.Fatal("AUTH_EXAMPLE_PORT must be between 1 and 65535")
		}
	}

	server := clayserver.NewServer(
		port,
		clayserver.WithRuntimeServeMuxOpts(
			runtime.WithMarshalerOption(
				runtime.MIMEWildcard, &runtime.JSONPb{
					MarshalOptions:   protojson.MarshalOptions{EmitUnpopulated: false},
					UnmarshalOptions: protojson.UnmarshalOptions{DiscardUnknown: true},
				},
			),
		),
		clayserver.WithHTTPMiddlewares(
			func(next http.Handler) http.Handler {
				bounded := http.MaxBytesHandler(next, 1<<20)

				return http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Cache-Control", "no-store")

						if r.URL.Path == "/" || r.URL.Path == "/app.js" || r.URL.Path == "/style.css" {
							name := r.URL.Path
							if name == "/" {
								name = "/index.html"
							}

							content, err := webFiles.ReadFile("web" + name)
							if err != nil {
								http.NotFound(w, r)
								return
							}

							http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(content))
							return
						}

						key := ""

						if cookie, err := r.Cookie("auth_example_caller"); err == nil {
							if _, exists := callers.Load(cookie.Value); exists {
								key = cookie.Value
							}
						}

						if key == "" {
							key = rand.Text()
							callers.Store(key, struct{}{})
							http.SetCookie(
								w,
								&http.Cookie{Name: "auth_example_caller", Value: key, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode},
							)
						}

						ctx := context.WithValue(
							r.Context(),
							callerContextKey{},
							authorization.Caller{Key: key},
						)
						bounded.ServeHTTP(w, r.WithContext(ctx))
					},
				)
			},
		),
		clayserver.WithGRPCUnaryMiddlewares(
			func(
				ctx context.Context,
				req any,
				_ *grpc.UnaryServerInfo,
				next grpc.UnaryHandler,
			) (any, error) {
				// Clay may also apply this interceptor to HTTP calls, which already
				// have a caller established by the HTTP middleware.
				if _, ok := ctx.Value(callerContextKey{}).(authorization.Caller); ok {
					return next(ctx, req)
				}

				md, _ := metadata.FromIncomingContext(ctx)
				request := &http.Request{Header: http.Header{"Cookie": md.Get("cookie")}}

				cookie, err := request.Cookie("auth_example_caller")
				if err != nil {
					return nil, status.Error(codes.Unauthenticated, "missing or unknown caller cookie")
				}

				if _, exists := callers.Load(cookie.Value); !exists {
					return nil, status.Error(codes.Unauthenticated, "missing or unknown caller cookie")
				}

				ctx = context.WithValue(ctx, callerContextKey{}, authorization.Caller{Key: cookie.Value})

				return next(ctx, req)
			},
		),
	)

	if err := server.Run(descriptor); err != nil {
		log.Fatal(err)
	}
}
