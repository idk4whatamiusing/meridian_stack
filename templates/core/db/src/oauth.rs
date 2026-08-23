use std::env;
use axum::{
    extract::{Query, State},
    http::{header, StatusCode},
    response::{IntoResponse, Redirect},
};
use jsonwebtoken::{decode, decode_header, Algorithm, DecodingKey, Validation};
use serde::{Deserialize, Serialize};

use url::Url;
use uuid::Uuid;

use crate::{AppState, session_cookie_string};

const AUTH_URL: &str = "https://accounts.google.com/o/oauth2/v2/auth";
const TOKEN_URL: &str = "https://oauth2.googleapis.com/token";
const CERTS_URL: &str = "https://www.googleapis.com/oauth2/v3/certs";
const JWKS_TTL: u64 = 3600;

#[derive(Deserialize)]
pub struct GoogleParams {
    client_id: String,
    client_secret: String,
    redirect_uri: String,
}

fn google_params() -> Option<GoogleParams> {
    Some(GoogleParams {
        client_id: env::var("GOOGLE_CLIENT_ID").ok()?,
        client_secret: env::var("GOOGLE_CLIENT_SECRET").ok()?,
        redirect_uri: env::var("GOOGLE_REDIRECT_URI").ok()?,
    })
}

#[derive(Deserialize)]
pub struct CallbackParams {
    code: String,
}

#[derive(Deserialize)]
struct TokenResponse {
    id_token: String,
}

#[derive(Debug, Serialize, Deserialize)]
struct IdClaims {
    sub: String,
    email: String,
    #[serde(default)]
    email_verified: bool,
}

pub async fn google_login() -> impl IntoResponse {
    let Some(p) = google_params() else {
        return Redirect::to("/api/auth/google/login?error=not_configured");
    };
    let mut url = Url::parse(AUTH_URL).expect("static url");
    url.query_pairs_mut()
        .append_pair("client_id", &p.client_id)
        .append_pair("redirect_uri", &p.redirect_uri)
        .append_pair("response_type", "code")
        .append_pair("scope", "openid email profile")
        .append_pair("prompt", "select_account");
    Redirect::to(url.as_str())
}

pub async fn google_callback(
    State(st): State<AppState>,
    Query(params): Query<CallbackParams>,
) -> impl IntoResponse {
    let Some(p) = google_params() else {
        return Redirect::to("/api/auth/google/login?error=not_configured").into_response();
    };

    let client = reqwest::Client::new();
    let res = match client
        .post(TOKEN_URL)
        .form(&[
            ("code", params.code.as_str()),
            ("client_id", p.client_id.as_str()),
            ("client_secret", p.client_secret.as_str()),
            ("redirect_uri", p.redirect_uri.as_str()),
            ("grant_type", "authorization_code"),
        ])
        .send()
        .await
    {
        Ok(r) => r,
        Err(_) => return Redirect::to("/?error=google_token").into_response(),
    };
    let Ok(tokens) = res.json::<TokenResponse>().await else {
        return Redirect::to("/?error=google_token").into_response();
    };

    let claims = match verify_id_token(&st, &tokens.id_token, &p.client_id).await {
        Ok(c) => c,
        Err(_) => return Redirect::to("/?error=google_invalid_token").into_response(),
    };
    if !claims.email_verified {
        return Redirect::to("/?error=google_email_unverified").into_response();
    }

    let id = Uuid::new_v4().to_string();
    let session_key = format!("session:{id}");
    st.cache.set(&session_key, &claims.email, 604800).await.ok();
    crate::upsert_user(&st.pool, &id, &claims.email).await;
    let app_url = env::var("APP_URL").unwrap_or_else(|_| "/".into());
    (
        StatusCode::FOUND,
        [(header::SET_COOKIE, session_cookie_string(&id))],
        Redirect::to(&app_url),
    )
        .into_response()
}

async fn verify_id_token(st: &AppState, token: &str, client_id: &str) -> Result<IdClaims, String> {
    let header = decode_header(token).map_err(|e| e.to_string())?;
    let kid = header.kid.ok_or("no kid")?;

    let jwks_json = match st.cache.get("google:jwks").await {
        Ok(Some(s)) => s,
        _ => {
            let body = reqwest::get(CERTS_URL).await.map_err(|e| e.to_string())?.text().await.map_err(|e| e.to_string())?;
            st.cache.set("google:jwks", &body, JWKS_TTL).await.ok();
            body
        }
    };
    let jwks: jsonwebtoken::jwk::JwkSet = serde_json::from_str(&jwks_json).map_err(|e| e.to_string())?;
    let jwk = jwks.find(&kid).ok_or("kid not in jwks")?;
    let key = DecodingKey::from_jwk(jwk).map_err(|e| e.to_string())?;

    let mut validation = Validation::new(Algorithm::RS256);
    validation.set_audience(&[client_id]);
    validation.set_issuer(&["https://accounts.google.com", "accounts.google.com"]);
    let data = decode::<IdClaims>(token, &key, &validation).map_err(|e| e.to_string())?;
    Ok(data.claims)
}