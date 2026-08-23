// Package clients - gRPC clients for the private mesh: db (Rust), realtime
// (Gleam), ai (hybrid). Every call carries x-backend-secret metadata.
package clients

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	aipb "github.com/idk4whatamiusing/meridian_stack/api/pb/aipb"
	dbpb "github.com/idk4whatamiusing/meridian_stack/api/pb/dbpb"
	realtimepb "github.com/idk4whatamiusing/meridian_stack/api/pb/realtimepb"
)

type Config struct {
	DBAddr       string
	RealtimeAddr string
	AiAddr       string
	Secret       string
}

type Clients struct {
	Secret   string
	DB       dbpb.DbClient
	Realtime realtimepb.RealtimeClient
	Ai       aipb.AiClient
}

func New(ctx context.Context, cfg Config) (*Clients, error) {
	dial := func(addr string) (*grpc.ClientConn, error) {
		return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	dbConn, err := dial(cfg.DBAddr)
	if err != nil {
		return nil, err
	}
	rtConn, err := dial(cfg.RealtimeAddr)
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
		Realtime: realtimepb.NewRealtimeClient(rtConn),
		Ai:       aipb.NewAiClient(aiConn),
	}, nil
}

// Ctx returns a context carrying the backend secret for one call.
func (c *Clients) Ctx(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "x-backend-secret", c.Secret)
}

type User = dbpb.User
