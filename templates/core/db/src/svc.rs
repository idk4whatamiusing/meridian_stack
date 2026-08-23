use sqlx::PgPool;
use tonic::{Request, Response, Status};
use uuid::Uuid;

use crate::pb::{
    db_server::Db,
    AppendChatMessageReply, AppendChatMessageRequest, ChatMessage, ChatSession,
    CreateChatSessionReply, CreateChatSessionRequest, DeleteChatSessionReply,
    DeleteChatSessionRequest, ListChatMessagesReply, ListChatMessagesRequest,
    ListChatSessionsReply, ListChatSessionsRequest, ListUsersReply, ListUsersRequest,
    RenameChatSessionReply, RenameChatSessionRequest, UpsertUserReply, UpsertUserRequest,
};

pub struct DbService {
    pool: PgPool,
    secret: String,
}

impl DbService {
    pub fn new(pool: PgPool, secret: String) -> Self {
        Self { pool, secret }
    }

    // every call must carry x-backend-secret - this service is private-network only
    fn authorize<T>(&self, req: &Request<T>) -> Result<(), Status> {
        let got = req
            .metadata()
            .get("x-backend-secret")
            .and_then(|v| v.to_str().ok())
            .unwrap_or("");
        if constant_time_eq(got.as_bytes(), self.secret.as_bytes()) {
            Ok(())
        } else {
            Err(Status::unauthenticated("bad backend secret"))
        }
    }
}

fn constant_time_eq(a: &[u8], b: &[u8]) -> bool {
    if a.len() != b.len() {
        return false;
    }
    a.iter().zip(b).fold(0u8, |acc, (x, y)| acc | (x ^ y)) == 0
}

#[tonic::async_trait]
impl Db for DbService {
    async fn upsert_user(
        &self,
        req: Request<UpsertUserRequest>,
    ) -> Result<Response<UpsertUserReply>, Status> {
        self.authorize(&req)?;
        let r = req.into_inner();
        let id = Uuid::parse_str(&r.id)
            .map_err(|_| Status::invalid_argument("id must be a uuid"))?;
        sqlx::query("INSERT INTO users (id, email) VALUES ($1, $2) ON CONFLICT (email) DO NOTHING")
            .bind(id)
            .bind(&r.email)
            .execute(&self.pool)
            .await
            .map_err(db_err)?;
        Ok(Response::new(UpsertUserReply { ok: true }))
    }

    async fn list_users(
        &self,
        req: Request<ListUsersRequest>,
    ) -> Result<Response<ListUsersReply>, Status> {
        self.authorize(&req)?;
        let limit = req.into_inner().limit.clamp(0, 100) as i64;
        let rows: Vec<(Uuid, String)> =
            sqlx::query_as("SELECT id, email FROM users ORDER BY created_at DESC LIMIT $1")
                .bind(limit)
                .fetch_all(&self.pool)
                .await
                .map_err(db_err)?;
        Ok(Response::new(ListUsersReply {
            users: rows
                .into_iter()
                .map(|(id, email)| crate::pb::User {
                    id: id.to_string(),
                    email,
                })
                .collect(),
        }))
    }

    async fn create_chat_session(
        &self,
        req: Request<CreateChatSessionRequest>,
    ) -> Result<Response<CreateChatSessionReply>, Status> {
        self.authorize(&req)?;
        let r = req.into_inner();
        let id = parse_uuid(&r.id)?;
        let user = parse_uuid(&r.user_id)?;
        let title = if r.title.is_empty() { "New chat".into() } else { r.title };
        sqlx::query("INSERT INTO chat_sessions (id, user_id, title) VALUES ($1, $2, $3)")
            .bind(id)
            .bind(user)
            .bind(title)
            .execute(&self.pool)
            .await
            .map_err(db_err)?;
        Ok(Response::new(CreateChatSessionReply { ok: true }))
    }

    async fn list_chat_sessions(
        &self,
        req: Request<ListChatSessionsRequest>,
    ) -> Result<Response<ListChatSessionsReply>, Status> {
        self.authorize(&req)?;
        let user = parse_uuid(&req.into_inner().user_id)?;
        let rows: Vec<(Uuid, String, bool, String)> = sqlx::query_as(
            "SELECT id, title, pinned, updated_at::text FROM chat_sessions \
             WHERE user_id = $1 ORDER BY pinned DESC, updated_at DESC",
        )
        .bind(user)
        .fetch_all(&self.pool)
        .await
        .map_err(db_err)?;
        Ok(Response::new(ListChatSessionsReply {
            sessions: rows
                .into_iter()
                .map(|(id, title, pinned, updated_at)| ChatSession {
                    id: id.to_string(),
                    title,
                    pinned,
                    updated_at,
                })
                .collect(),
        }))
    }

    async fn rename_chat_session(
        &self,
        req: Request<RenameChatSessionRequest>,
    ) -> Result<Response<RenameChatSessionReply>, Status> {
        self.authorize(&req)?;
        let r = req.into_inner();
        sqlx::query("UPDATE chat_sessions SET title = $3 WHERE id = $1 AND user_id = $2")
            .bind(parse_uuid(&r.id)?)
            .bind(parse_uuid(&r.user_id)?)
            .bind(r.title)
            .execute(&self.pool)
            .await
            .map_err(db_err)?;
        Ok(Response::new(RenameChatSessionReply { ok: true }))
    }

    async fn delete_chat_session(
        &self,
        req: Request<DeleteChatSessionRequest>,
    ) -> Result<Response<DeleteChatSessionReply>, Status> {
        self.authorize(&req)?;
        let r = req.into_inner();
        sqlx::query("DELETE FROM chat_sessions WHERE id = $1 AND user_id = $2")
            .bind(parse_uuid(&r.id)?)
            .bind(parse_uuid(&r.user_id)?)
            .execute(&self.pool)
            .await
            .map_err(db_err)?;
        Ok(Response::new(DeleteChatSessionReply { ok: true }))
    }

    async fn append_chat_message(
        &self,
        req: Request<AppendChatMessageRequest>,
    ) -> Result<Response<AppendChatMessageReply>, Status> {
        self.authorize(&req)?;
        let r = req.into_inner();
        if r.role != "user" && r.role != "assistant" {
            return Err(Status::invalid_argument("role must be user|assistant"));
        }
        let mut tx = self.pool.begin().await.map_err(db_err)?;
        sqlx::query(
            "INSERT INTO chat_messages (session_id, role, content) \
             SELECT $1, $3, $4 WHERE EXISTS (SELECT 1 FROM chat_sessions WHERE id = $1 AND user_id = $2)",
        )
        .bind(parse_uuid(&r.session_id)?)
        .bind(parse_uuid(&r.user_id)?)
        .bind(&r.role)
        .bind(&r.content)
        .execute(&mut *tx)
        .await
        .map_err(db_err)?;
        sqlx::query("UPDATE chat_sessions SET updated_at = now() WHERE id = $1")
            .bind(parse_uuid(&r.session_id)?)
            .execute(&mut *tx)
            .await
            .map_err(db_err)?;
        tx.commit().await.map_err(db_err)?;
        Ok(Response::new(AppendChatMessageReply { ok: true }))
    }

    async fn list_chat_messages(
        &self,
        req: Request<ListChatMessagesRequest>,
    ) -> Result<Response<ListChatMessagesReply>, Status> {
        self.authorize(&req)?;
        let r = req.into_inner();
        let session = parse_uuid(&r.session_id)?;
        let user = parse_uuid(&r.user_id)?;
        let rows: Vec<(String, String, String)> = if r.limit <= 0 {
            sqlx::query_as(
                "SELECT m.role, m.content, m.created_at::text FROM chat_messages m \
                 JOIN chat_sessions s ON s.id = m.session_id \
                 WHERE m.session_id = $1 AND s.user_id = $2 ORDER BY m.created_at ASC",
            )
            .bind(session)
            .bind(user)
            .fetch_all(&self.pool)
            .await
            .map_err(db_err)?
        } else {
            sqlx::query_as(
                "SELECT m.role, m.content, m.created_at::text FROM chat_messages m \
                 JOIN chat_sessions s ON s.id = m.session_id \
                 WHERE m.session_id = $1 AND s.user_id = $2 ORDER BY m.created_at ASC LIMIT $3",
            )
            .bind(session)
            .bind(user)
            .bind(r.limit as i64)
            .fetch_all(&self.pool)
            .await
            .map_err(db_err)?
        };
        Ok(Response::new(ListChatMessagesReply {
            messages: rows
                .into_iter()
                .map(|(role, content, created_at)| ChatMessage { role, content, created_at })
                .collect(),
        }))
    }
}

fn parse_uuid(s: &str) -> Result<Uuid, Status> {
    Uuid::parse_str(s).map_err(|_| Status::invalid_argument("expected uuid"))
}

fn db_err(e: sqlx::Error) -> Status {
    tracing::warn!("db error: {e}");
    Status::internal("database error")
}
