
use async_graphql::{Context, EmptyMutation, EmptySubscription, Error, Object, Schema, SimpleObject};
use axum::http::HeaderMap;
use serde_json::json;

use crate::cache::Cache;

#[derive(Clone)]
pub struct HttpHeaders(pub HeaderMap);

#[derive(SimpleObject)]
struct User {
    id: String,
    email: String,
}

pub type AppSchema = Schema<QueryRoot, EmptyMutation, EmptySubscription>;

pub struct QueryRoot;

#[Object]
impl QueryRoot {
    async fn me(&self, ctx: &Context<'_>) -> Result<Option<User>, Error> {
        let cache = ctx.data::<Cache>()?;
        let hdrs = ctx.data_opt::<HttpHeaders>().map(|h| h.0.clone());
        let Some(user) = crate::auth_user(cache, hdrs.as_ref()).await else {
            return Ok(None);
        };
        let (id, email) = user;
        Ok(Some(User { id, email }))
    }

    async fn users(&self, ctx: &Context<'_>, #[graphql(default = 50)] limit: i32) -> Result<Vec<User>, Error> {
        let pool = ctx.data::<sqlx::PgPool>()?;
        let rows = sqlx::query_as::<_, (uuid::Uuid, String)>(
            "SELECT id, email FROM users ORDER BY created_at DESC LIMIT $1",
        )
        .bind(limit.max(0).min(100) as i64)
        .fetch_all(pool)
        .await?;
        Ok(rows.into_iter().map(|(id, email)| User { id: id.to_string(), email }).collect())
    }

    async fn health(&self, ctx: &Context<'_>) -> Result<String, Error> {
        let pool = ctx.data::<sqlx::PgPool>()?;
        let cache = ctx.data::<Cache>()?;
        sqlx::query("SELECT 1").execute(pool).await?;
        Ok(json!({ "status": "ok", "cache": cache.kind() }).to_string())
    }
}

pub fn build(cache: Cache, pool: sqlx::PgPool) -> AppSchema {
    Schema::build(QueryRoot, EmptyMutation, EmptySubscription)
        .data(cache)
        .data(pool)
        .finish()
}