package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/idk4whatamiusing/meridian_stack/ai/internal/providers"
	"github.com/idk4whatamiusing/meridian_stack/ai/internal/rag"
	aipb "github.com/idk4whatamiusing/meridian_stack/api/pb/aipb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

const (
	supportThreshold = 0.92 // knowledge-base answers repeat a lot
	chatThreshold    = 0.95
	topK             = 5
)

type server struct {
	aipb.UnimplementedAiServer
	rag *rag.Client
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func sourcesFromRAG(in []rag.Source) []*aipb.Source {
	out := make([]*aipb.Source, len(in))
	for i, s := range in {
		out[i] = &aipb.Source{Id: s.Id, Title: deref(s.Title), Text: s.Text, Score: s.Score}
	}
	return out
}

func sourcesHash(in []rag.Source) string {
	h := ""
	for _, s := range in {
		h += s.Id + "|"
	}
	return fmt.Sprintf("%x", h)
}

func (s *server) chatMessages(message, system string, useRAG bool) []providers.Message {
	if !useRAG {
		msgs := []providers.Message{}
		if system != "" {
			msgs = append(msgs, providers.Message{Role: "system", Content: system})
		}
		return append(msgs, providers.Message{Role: "user", Content: message})
	}
	return []providers.Message{{Role: "user", Content: message}}
}

func (s *server) Chat(ctx context.Context, req *aipb.ChatRequest) (*aipb.ChatReply, error) {
	kind, threshold := "chat", chatThreshold
	collection := "chat"
	if req.GetProvider() == "support" {
		kind, threshold, collection = "support", supportThreshold, "support"
	}
	user := req.GetUserId()

	if hit := s.rag.CacheLookup(kind, user, req.GetMessage(), threshold); hit.Hit {
		return &aipb.ChatReply{Reply: hit.Answer, Model: "cache", Cached: true}, nil
	}

	var sources []rag.Source
	var ctxText string
	if req.GetUseRag() {
		srcs, err := s.rag.Retrieve(req.GetMessage(), collection, topK)
		if err == nil && len(srcs) > 0 {
			sources = srcs
			var b strings.Builder
			b.WriteString("Answer ONLY using these sources:\n")
			for _, sc := range srcs {
				fmt.Fprintf(&b, "\n[%s]\n%s\n", deref(sc.Title), sc.Text)
			}
			ctxText = b.String()
		}
	}

	gen, model := providers.Provider(context.Background())
	msgs := buildMsgs(systemOrDefault(req.GetSystem()), ctxText, req.GetMessage())
	stream, err := gen(msgs)
	if err != nil {
		return nil, err
	}
	var reply strings.Builder
	for c := range stream {
		reply.WriteString(c.Delta)
	}
	ans := reply.String()
	s.rag.CacheStore(kind, user, req.GetMessage(), ans, sourcesHash(sources))
	return &aipb.ChatReply{
		Reply:   ans,
		Model:   model,
		Sources: sourcesFromRAG(sources),
	}, nil
}

func (s *server) ChatStream(req *aipb.ChatRequest, stream aipb.Ai_ChatStreamServer) error {
	rep, err := s.Chat(stream.Context(), req)
	if err != nil {
		return err
	}
	if rep.Cached {
		_ = stream.Send(&aipb.ChatChunk{Delta: rep.Reply, Done: true, Cached: true})
		return nil
	}
	gen, _ := providers.Provider(context.Background())
	msgs := buildMsgs(systemOrDefault(req.GetSystem()), "", req.GetMessage())
	ch, err := gen(msgs)
	if err != nil {
		return err
	}
	first := true
	for c := range ch {
		ck := &aipb.ChatChunk{Delta: c.Delta, Done: c.Done}
		if first {
			ck.Sources = rep.Sources // RAG ran before streaming; attach on first chunk
			first = false
		}
		if err := stream.Send(ck); err != nil {
			return err
		}
		if c.Done {
			break
		}
	}
	s.rag.CacheStore("chat", req.GetUserId(), req.GetMessage(), rep.Reply, "")
	return nil
}

func (s *server) SupportQuery(req *aipb.SupportQueryRequest, stream aipb.Ai_SupportQueryServer) error {
	msg := req.GetMessage()
	user := req.GetUserId()

	if hit := s.rag.CacheLookup("support", user, msg, supportThreshold); hit.Hit {
		return stream.Send(&aipb.SupportChunk{Delta: hit.Answer, Done: true, Cached: true})
	}

	sources, err := s.rag.Retrieve(msg, "support", topK)
	if err != nil || len(sources) == 0 {
		return stream.Send(&aipb.SupportChunk{
			Delta: "I could not find anything in the knowledge base for that. Try rephrasing or contact a human.",
			Done:  true,
		})
	}

	var b strings.Builder
	b.WriteString("You are a support assistant. Answer ONLY from the knowledge base below. If it is not covered, say you don't know.\n\nKnowledge base:\n")
	for _, sc := range sources {
		fmt.Fprintf(&b, "\n[%s]\n%s\n", deref(sc.Title), sc.Text)
	}

	llmURL := envOr("SUPPORT_LLM_URL", "http://localhost:8081") + "/v1"
	openaiGen := providers.OpenAIStream(llmURL, envOr("SUPPORT_LLM_KEY", ""), envOr("SUPPORT_LLM_MODEL", "tinyllama"))
	ch, err := openaiGen([]providers.Message{
		{Role: "system", Content: b.String()},
		{Role: "user", Content: msg},
	})
	if err != nil {
		return stream.Send(&aipb.SupportChunk{
			Delta: "Support LLM is not reachable (" + err.Error() + "). Start it with `make support-llm`.",
			Done:  true,
		})
	}
	var answer strings.Builder
	first := true
	for c := range ch {
		ck := &aipb.SupportChunk{Delta: c.Delta, Done: c.Done}
		if first {
			ck.Sources = sourcesFromRAG(sources)
			first = false
		}
		answer.WriteString(c.Delta)
		if err := stream.Send(ck); err != nil {
			return err
		}
		if c.Done {
			break
		}
	}
	s.rag.CacheStore("support", user, msg, answer.String(), sourcesHash(sources))
	return nil
}

func (s *server) Ingest(ctx context.Context, req *aipb.IngestRequest) (*aipb.IngestReply, error) {
	n, err := s.rag.IngestFull(req.GetDocuments(), collectionOr(req.GetCollection()))
	if err != nil {
		return nil, err
	}
	return &aipb.IngestReply{Chunks: int32(n)}, nil
}

func (s *server) Predict(ctx context.Context, req *aipb.PredictRequest) (*aipb.PredictReply, error) {
	t := strings.ToLower(req.GetText())
	switch {
	case containsAny(t, "bad", "terrible", "hate", "awful"):
		return &aipb.PredictReply{Label: "negative"}, nil
	case containsAny(t, "great", "love", "good", "nice"):
		return &aipb.PredictReply{Label: "positive"}, nil
	default:
		return &aipb.PredictReply{Label: "neutral"}, nil
	}
}

// ---- small helpers ----

func buildMsgs(system, ragContext, message string) []providers.Message {
	var msgs []providers.Message
	sys := system
	if ragContext != "" {
		if sys != "" {
			sys += "\n\n" + ragContext
		} else {
			sys = ragContext
		}
	}
	if sys != "" {
		msgs = append(msgs, providers.Message{Role: "system", Content: sys})
	}
	return append(msgs, providers.Message{Role: "user", Content: message})
}

func systemOrDefault(s string) string {
	if s == "" {
		return "You are a concise assistant."
	}
	return s
}

func collectionOr(c string) string {
	if c == "" {
		return "support"
	}
	return c
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func containsAny(t string, words ...string) bool {
	for _, w := range words {
		if strings.Contains(t, w) {
			return true
		}
	}
	return false
}

func main() {
	go func() { // plain-HTTP health for compose probes
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
		_ = http.ListenAndServe(":8081", mux)
	}()

	grpcAddr := envOr("AI_GRPC_LISTEN", ":8002")
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("ai gRPC listening on %s (rag sidecar: %s)", grpcAddr, envOr("RAG_URL", "http://localhost:8003"))

	s := &server{rag: rag.New(envOr("RAG_URL", "http://localhost:8003"))}
	gs := grpc.NewServer()
	aipb.RegisterAiServer(gs, s)
	hs := health.NewServer()
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(gs, hs)
	if err := gs.Serve(lis); err != nil {
		log.Fatal(err)
	}
}
