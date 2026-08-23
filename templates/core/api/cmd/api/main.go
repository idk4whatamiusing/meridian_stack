// Meridian api - the public Go service.
// Browser talks GraphQL here (raw fetch + graphql-ws); this service owns
// Redis (sessions, 7d caches, chat-history fast path), Google OAuth, and
// fans gRPC calls out to db (Rust), realtime (Gleam) and ai (hybrid).
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/idk4whatamiusing/meridian_stack/api/graph"
	"github.com/idk4whatamiusing/meridian_stack/api/internal/clients"
	"github.com/idk4whatamiusing/meridian_stack/api/internal/hub"
	"github.com/idk4whatamiusing/meridian_stack/api/internal/oauth"
	"github.com/idk4whatamiusing/meridian_stack/api/internal/store"
	"github.com/redis/go-redis/v9"
)

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	ctx := context.Background()
	port := envOr("PORT", "8000")
	appURL := envOr("APP_URL", "/")
	secret := envOr("BACKEND_SECRET", "change-me")

	rdb := redis.NewClient(&redis.Options{Addr: envOr("REDIS_ADDR", "localhost:6379")})
	st := store.New(rdb, 7*24*time.Hour) // every cache = 7 days
	cl, err := clients.New(ctx, clients.Config{
		DBAddr:      envOr("DB_GRPC_ADDR", "localhost:8010"),
		RealtimeURL: envOr("REALTIME_URL", "http://localhost:8001"),
		AiAddr:      envOr("AI_GRPC_ADDR", "localhost:8002"),
		Secret:      secret,
	})
	if err != nil {
		log.Fatalf("grpc dial: %v", err)
	}
	hb := hub.New(256)

	o := &oauth.OAuth{Store: st, Clients: cl, AppURL: appURL}
	resolver := &graph.Resolver{Store: st, Clients: cl, Hub: hb}

	srv := handler.NewDefaultServer(graph.NewExecutableSchema(graph.Config{Resolvers: resolver}))
	srv.AddTransport(&transport.Websocket{
		InitFunc: func(ctx context.Context, init transport.InitPayload) (context.Context, *transport.InitPayload, error) {
			// graphql-ws clients authenticate by sending their session id as
			// connection_params: { session: "<cookie value>" }
			if sid, _ := init["session"].(string); sid != "" {
				if email, err := st.SessionEmail(ctx, sid); err == nil {
					return oauth.WithUser(ctx, sid, email), nil, nil
				}
			}
			return ctx, nil, nil // anonymous subscriptions are allowed
		},
	})

	gql := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		req = req.WithContext(oauth.WithWriter(req.Context(), w))
		if sid, err := req.Cookie("session"); err == nil {
			if email, rerr := st.SessionEmail(req.Context(), sid.Value); rerr == nil {
				req = req.WithContext(oauth.WithUser(req.Context(), sid.Value, email))
			}
		}
		srv.ServeHTTP(w, req)
	})

	r := chi.NewRouter()
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "x-backend-secret"},
		AllowCredentials: true,
	}))

	r.Get("/api/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	r.Get("/api/auth/google", o.Login)
	r.Get("/api/auth/google/callback", o.Callback)
	r.Handle("/api/graphql", gql)
	r.Get("/api/graphql/playground", playground.Handler("Meridian", "/api/graphql"))

	log.Printf("api listening on :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}
