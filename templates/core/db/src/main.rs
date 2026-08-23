mod pb {
    tonic::include_proto!("meridian.db.v1");
}

mod svc;

use sqlx::postgres::PgPoolOptions;

#[tokio::main]
async fn main() {
    dotenvy::dotenv().ok();
    tracing_subscriber::fmt().with_env_filter(
        tracing_subscriber::EnvFilter::try_from_default_env().unwrap_or_else(|_| "db=info,tower=info".into()),
    ).init();

    let database_url =
        std::env::var("DATABASE_URL").unwrap_or_else(|_| "postgres://app:app@localhost:5432/app".into());
    let pool = PgPoolOptions::new()
        .max_connections(5)
        .connect(&database_url)
        .await
        .expect("connect to postgres (docker compose up -d)");
    sqlx::migrate!("./migrations").run(&pool).await.expect("run migrations");

    // plain-HTTP health for compose/caddy probes (gRPC port is :8010)
    tokio::spawn(async move {
        let app = axum::Router::new()
            .route("/healthz", axum::routing::get(|| async { "ok" }))
            .layer(tower_http::trace::TraceLayer::new_for_http());
        let listener = tokio::net::TcpListener::bind("0.0.0.0:8011").await.unwrap();
        tracing::info!("healthz on :8011");
        axum::serve(listener, app).await.unwrap();
    });

    let secret = std::env::var("BACKEND_SECRET").unwrap_or_else(|_| "change-me".into());
    let addr = "0.0.0.0:8010".parse().unwrap();
    tracing::info!("db gRPC listening on 0.0.0.0:8010");

    tonic::transport::Server::builder()
        .add_service(pb::db_server::DbServer::new(svc::DbService::new(pool, secret)))
        .serve(addr)
        .await
        .unwrap();
}
