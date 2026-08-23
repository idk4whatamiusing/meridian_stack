// Package clients - private mesh callers: db (Rust, gRPC), ai (hybrid, gRPC),
// realtime (Gleam - notified over its existing secret-gated /broadcast hook;
// the gRPC contract lives in packages/proto/realtime.proto for the upgrade).
package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	aipb "github.com/idk4whatamiusing/meridian_stack/api/pb/aipb"
	dbpb "github.com/idk4whatamiusing/meridian_stack/api/pb/dbpb"
)

type Config struct {
	DBAddr      string
	RealtimeURL string // e.g. http://realtime:8001
	AiAddr      string
	Secret      string
}

type Clients struct {
	Secret   string
	DB       dbpb.DbClient
	Ai       aipb.AiClient
	realtime *http.Client
	base     string
}

func New(ctx context.Context, cfg Config) (*Clients, error) {
	dial := func(addr string) (*grpc.ClientConn, error) {
		return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	dbConn, err := dial(cfg.DBAddr)
	if err != nil {
		return nil, err
	}
	aiConn, err := dial(cfg.AiAddr)
	if err != nil {
		return nil, err
	}
	return &Clients{
		Secret:   cfg.Secret,
		DB:       dbpb.NewDbClient(dbConn),
		Ai:       aipb.NewAiClient(aiConn),
		realtime: &http.Client{Timeout: 5 * time.Second},
		base:     cfg.RealtimeURL,
	}, nil
}

// Ctx returns a context carrying the backend secret for one gRPC call.
func (c *Clients) Ctx(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "x-backend-secret", c.Secret)
}

// NotifyRealtime fires fanout #2 (parallel, best-effort).
func (c *Clients) NotifyRealtime(message string) {
	go func() {
		body, _ := json.Marshal(map[string]string{"message": message})
		req, err := http.NewRequest(http.MethodPost, c.base+"/broadcast", bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-backend-secret", c.Secret)
		resp, err := c.realtime.Do(req)
		if err != nil {
			fmt.Printf("realtime notify: %v\n", err)
			return
		}
		resp.Body.Close()
	}()
}
