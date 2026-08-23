// Package rag - client for the Python RAG sidecar (localhost:8003).
// The sidecar owns embeddings, pgvector retrieval and the semantic cache.
package rag

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Source struct {
	Id    string  `json:"id"`
	Title *string `json:"title"`
	Text  string  `json:"text"`
	Score float32 `json:"score"`
}

type Client struct {
	base string
	hc   *http.Client
}

func New(base string) *Client {
	return &Client{base: base, hc: &http.Client{Timeout: 60 * time.Second}}
}

func (c *Client) post(path string, in any, out any) error {
	body, _ := json.Marshal(in)
	resp, err := c.hc.Post(c.base+path, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("rag %s: %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) Retrieve(query, collection string, k int) ([]Source, error) {
	var out struct {
		Sources []Source `json:"sources"`
	}
	err := c.post("/retrieve", map[string]any{
		"query": query, "collection": collection, "k": k,
	}, &out)
	return out.Sources, err
}

func (c *Client) IngestFull(documents []string, collection string) (int, error) {
	var out struct {
		Chunks int `json:"chunks"`
	}
	err := c.post("/ingest", map[string]any{"documents": documents, "collection": collection}, &out)
	return out.Chunks, err
}

type Lookup struct {
	Hit         bool    `json:"hit"`
	Answer      string  `json:"answer"`
	SourcesHash string  `json:"sources_hash"`
	Score       float64 `json:"score"`
}

func (c *Client) CacheLookup(kind, userID, text string, threshold float64) Lookup {
	var out Lookup
	_ = c.post("/cache_lookup", map[string]any{
		"kind": kind, "user_id": userID, "text": text, "threshold": threshold,
	}, &out)
	return out
}

func (c *Client) CacheStore(kind, userID, text, answer, sourcesHash string) {
	_ = c.post("/cache_store", map[string]any{
		"kind": kind, "user_id": userID, "text": text,
		"answer": answer, "sources_hash": sourcesHash,
	}, &map[string]any{})
}
