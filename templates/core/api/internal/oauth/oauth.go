// Package oauth - Google login (AWS path; the Cloudflare gateway has its own).
package oauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/idk4whatamiusing/meridian_stack/api/internal/clients"
	"github.com/idk4whatamiusing/meridian_stack/api/internal/store"
	dbpb "github.com/idk4whatamiusing/meridian_stack/api/pb/dbpb"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type ctxKey int

const (
	userIDKey ctxKey = iota
	userEmailKey
	writerKey
)

// Writer plumbing: GraphQL resolvers need the http.ResponseWriter to set cookies.
func WithWriter(ctx context.Context, w http.ResponseWriter) context.Context {
	return context.WithValue(ctx, writerKey, w)
}

func setCookie(ctx context.Context, cookie *http.Cookie) {
	if w, ok := ctx.Value(writerKey).(http.ResponseWriter); ok {
		http.SetCookie(w, cookie)
	}
}

// SetSessionCookie logs the browser in from a resolver.
func SetSessionCookie(ctx context.Context, id string) {
	setCookie(ctx, &http.Cookie{
		Name: "session", Value: id, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: int((7 * 24 * time.Hour).Seconds()),
	})
}

// ClearSessionCookie logs the browser out from a resolver.
func ClearSessionCookie(ctx context.Context) {
	setCookie(ctx, &http.Cookie{Name: "session", Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
}

// WithUser injects the authenticated user into a request context.
func WithUser(ctx context.Context, id, email string) context.Context {
	ctx = context.WithValue(ctx, userIDKey, id)
	return context.WithValue(ctx, userEmailKey, email)
}

func UserID(ctx context.Context) string {
	v, _ := ctx.Value(userIDKey).(string)
	return v
}

func UserEmail(ctx context.Context) string {
	v, _ := ctx.Value(userEmailKey).(string)
	return v
}

type OAuth struct {
	Store   *store.Store
	Clients *clients.Clients
	AppURL  string
}

func (o *OAuth) config(r *http.Request) *oauth2.Config {
	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	host := r.Host // behind caddy this is api.DOMAIN
	return &oauth2.Config{
		ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		RedirectURL:  scheme + "://" + host + "/api/auth/google/callback",
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     google.Endpoint,
	}
}

func randState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (o *OAuth) Login(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("GOOGLE_CLIENT_ID") == "" {
		http.Redirect(w, r, "/?error=google_not_configured", http.StatusFound)
		return
	}
	state := randState()
	http.SetCookie(w, &http.Cookie{
		Name: "oauth_state", Value: state, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: 600,
	})
	http.Redirect(w, r, o.config(r).AuthCodeURL(state), http.StatusFound)
}

func (o *OAuth) Callback(w http.ResponseWriter, r *http.Request) {
	stCookie, err := r.Cookie("oauth_state")
	if err != nil || stCookie.Value == "" || stCookie.Value != r.URL.Query().Get("state") {
		http.Redirect(w, r, "/?error=google_state", http.StatusFound)
		return
	}
	tok, err := o.config(r).Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		log.Printf("oauth exchange: %v", err)
		http.Redirect(w, r, "/?error=google_token", http.StatusFound)
		return
	}

	provider, err := oidc.NewProvider(r.Context(), "https://accounts.google.com")
	if err != nil {
		http.Redirect(w, r, "/?error=google_provider", http.StatusFound)
		return
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: os.Getenv("GOOGLE_CLIENT_ID")})
	idToken, err := verifier.Verify(r.Context(), tok.Extra("id_token").(string))
	if err != nil {
		http.Redirect(w, r, "/?error=google_invalid_token", http.StatusFound)
		return
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := idToken.Claims(&claims); err != nil || !claims.EmailVerified || claims.Email == "" {
		http.Redirect(w, r, "/?error=google_email_unverified", http.StatusFound)
		return
	}

	id := uuid.NewString()
	if err := o.Store.CreateSession(r.Context(), id, claims.Email); err != nil {
		http.Redirect(w, r, "/?error=session", http.StatusFound)
		return
	}
	if _, err := o.Clients.DB.UpsertUser(o.Clients.Ctx(r.Context()), &dbpb.UpsertUserRequest{Id: id, Email: claims.Email}); err != nil {
		log.Printf("upsert user: %v", err)
	}
	http.SetCookie(w, &http.Cookie{
		Name: "session", Value: id, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: int((7 * 24 * time.Hour).Seconds()),
	})
	target := o.AppURL
	if target == "" {
		target = "/"
	}
	http.Redirect(w, r, target, http.StatusFound)
}
