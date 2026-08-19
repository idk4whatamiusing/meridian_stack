use std::env;

use async_graphql_axum::{GraphQLRequest, GraphQLResponse};
use axum::{
    extract::ws::{Message, WebSocket, WebSocketUpgrade},
    extract::State,
    http::{header, HeaderMap, StatusCode},
    response::sse::{Event, Sse},
    response::IntoResponse,
    routing::{get, post},
    Json, Router,
};
use futures_util::{SinkExt, StreamExt};
use serde_json::{json, Value};
use sqlx::postgres::PgPoolOptions;
use tokio::sync::broadcast;
use tokio_stream::wrappers::BroadcastStream;
use tower_http::{cors::CorsLayer, trace::TraceLayer};
use uuid::Uuid;

mod cache;
mod graphql;
mod oauth;

#[derive(Clone)]
struct AppState {
    cache: cache::Cache,
    pool: sqlx::PgPool,
    tx: broadcast::Sender<String>,
    secret: String,
    schema: graphql::AppSchema,
}

fn env_or(key: &str, default: &str) -> String {
    env::var(key).unwrap_or_else(|_| default.to_string())
}

fn bearer_ok(secret: &str, headers: &HeaderMap) -> bool {
    headers
        .get("x-backend-secret")
        .is_some_and(|v| v.to_str().unwrap_or("") == secret)
}

fn session_cookie(headers: &HeaderMap) -> Option<String> {
    headers
        .get(header::COOKIE)?
        .to_str()
        .ok()?
        .split(';')
        .map(str::trim)
        .find_map(|c| c.strip_prefix("session="))
        .map(|s| s.to_string())
}

fn session_cookie_string(id: &str) -> String {
    format!("session={id}; Path=/; HttpOnly; SameSite=Lax; Max-Age=604800")
}

async fn auth_user(cache: &cache::Cache, headers: Option<&HeaderMap>) -> Option<(String, String)> {
    let headers = headers?;
    // Cloudflare gateway path: it validates the session and forwards the user id.
    if let Some(id) = headers.get("x-user-id").and_then(|v| v.to_str().ok()) {
        return Some((id.to_string(), String::new()));
    }
    // AWS path: session cookie -> Redis lookup.
    let id = session_cookie(headers)?;
    let email = cache.get(&format!("session:{id}")).await.ok().flatten()?;
    Some((id, email))
}

async fn dev_login(State(st): State<AppState>, Json(body): Json<Value>) -> impl IntoResponse {
    let email = body
        .get("email")
        .and_then(|e| e.as_str())
        .unwrap_or("dev@example.com");
    let id = Uuid::new_v4().to_string();
    let session_key = format!("session:{id}");
    st.cache.set(&session_key, email, 604800).await.ok();
    (
        StatusCode::OK,
        [(header::SET_COOKIE, session_cookie_string(&id))],
        Json(json!({ "ok": true, "user": { "id": id, "email": email } })),
    )
}

async fn logout(State(st): State<AppState>, headers: HeaderMap) -> impl IntoResponse {
    if let Some(id) = session_cookie(&headers) {
        st.cache.delete(&format!("session:{id}")).await.ok();
    }
    (
        StatusCode::OK,
        [(
            header::SET_COOKIE,
            "session=; Path=/; HttpOnly; Max-Age=0".to_string(),
        )],
        Json(json!({ "ok": true })),
    )
}

async fn me(State(st): State<AppState>, headers: HeaderMap) -> Result<Json<Value>, (StatusCode, Json<Value>)> {
    match auth_user(&st.cache, Some(&headers)).await {
        Some((id, email)) => Ok(Json(json!({ "user": { "id": id, "email": email } }))),
        None => Err((StatusCode::UNAUTHORIZED, Json(json!({ "error": "unauthorized" })))),
    }
}

async fn graphql_handler(
    State(st): State<AppState>,
    headers: HeaderMap,
    req: GraphQLRequest,
) -> GraphQLResponse {
    let mut req = req.into_inner();
    req = req.data(graphql::HttpHeaders(headers));
    st.schema.execute(req).await.into()
}

async fn graphiql() -> impl IntoResponse {
    let html = async_graphql::http::GraphiQLSource::build()
        .endpoint("/api/graphql")
        .title("Template API")
        .finish();
    (StatusCode::OK, [(header::CONTENT_TYPE, "text/html")], html)
}

async fn health(State(st): State<AppState>) -> impl IntoResponse {
    let db = match sqlx::query("SELECT 1").execute(&st.pool).await {
        Ok(_) => "ok",
        Err(_) => "down",
    };
    Json(json!({ "status": "ok", "db": db, "cache": st.cache.kind() }))
}

async fn events(State(st): State<AppState>) -> Sse<impl futures_util::Stream<Item = Result<Event, std::io::Error>>> {
    let stream = BroadcastStream::new(st.tx.subscribe())
        .map(|r| r.ok().map(|msg| Ok(Event::default().data(msg))))
        .filter_map(futures_util::future::ready);
    Sse::new(stream)
}

async fn broadcast(
    State(st): State<AppState>,
    headers: HeaderMap,
    Json(body): Json<Value>,
) -> impl IntoResponse {
    if !bearer_ok(&st.secret, &headers) {
        return (StatusCode::UNAUTHORIZED, Json(json!({ "error": "bad secret" })));
    }
    let msg = body
        .get("message")
        .and_then(|m| m.as_str())
        .unwrap_or("ping")
        .to_string();
    let _ = st.tx.send(format!("api: {msg}"));
    (StatusCode::OK, Json(json!({ "ok": true })))
}

async fn ws(ws: WebSocketUpgrade, State(st): State<AppState>) -> impl IntoResponse {
    ws.on_upgrade(move |socket| handle_ws(socket, st.tx.clone()))
}

async fn handle_ws(socket: WebSocket, tx: broadcast::Sender<String>) {
    let (mut sink, mut stream) = socket.split();
    while let Some(Ok(msg)) = stream.next().await {
        if let Message::Text(text) = msg {
            let _ = sink.send(Message::Text(text.clone())).await;
            let _ = tx.send(format!("ws: {text}"));
        }
    }
}

#[tokio::main]
async fn main() {
    dotenvy::dotenv().ok();
    tracing_subscriber::fmt::init();

    let database_url =
        env_or("DATABASE_URL", "postgres://app:app@localhost:5432/app");
    let pool = PgPoolOptions::new()
        .max_connections(5)
        .connect(&database_url)
        .await
        .expect("connect to postgres (docker compose up -d)");
    sqlx::migrate!("./migrations")
        .run(&pool)
        .await
        .expect("run migrations");

    let cache = cache::Cache::from_env();
    let (tx, _) = broadcast::channel(256);
    let state = AppState {
        cache: cache.clone(),
        pool: pool.clone(),
        tx,
        secret: env_or("BACKEND_SECRET", "change-me"),
        schema: graphql::build(cache, pool),
    };

    let app = Router::new()
        .route("/api/health", get(health))
        .route("/api/me", get(me))
        .route("/api/auth/dev-login", post(dev_login))
        .route("/api/auth/logout", post(logout))
        .route("/api/auth/google", get(oauth::google_login))
        .route("/api/auth/google/callback", get(oauth::google_callback))
        .route("/api/graphql", get(graphiql).post(graphql_handler))
        .route("/api/events", get(events))
        .route("/api/broadcast", post(broadcast))
        .route("/api/ws", get(ws))
        .with_state(state)
        .layer(CorsLayer::permissive()) // ponytail: permissive CORS, restrict to your domain before prod
        .layer(TraceLayer::new_for_http());

    let addr = format!("0.0.0.0:{}", env_or("PORT", "8000"));
    let listener = tokio::net::TcpListener::bind(&addr).await.unwrap();
    tracing::info!("api listening on {addr}");
    axum::serve(listener, app).await.unwrap();
}