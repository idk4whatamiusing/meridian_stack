use std::env;

#[derive(Clone)]
pub struct RedisCache {
    client: redis::Client,
}

impl RedisCache {
    pub fn new(url: &str) -> Self {
        Self {
            client: redis::Client::open(url).expect("valid redis url"),
        }
    }
}

pub struct KvCache {
    account: String,
    namespace: String,
    token: String,
    client: reqwest::Client,
}

impl KvCache {
    pub fn new(account: &str, namespace: &str, token: &str) -> Self {
        Self {
            account: account.to_string(),
            namespace: namespace.to_string(),
            token: token.to_string(),
            client: reqwest::Client::new(),
        }
    }

    fn url(&self, key: &str) -> String {
        format!(
            "https://api.cloudflare.com/client/v4/accounts/{}/storage/kv/namespaces/{}/values/{}",
            self.account, self.namespace, key
        )
    }
}

#[derive(Clone)]
pub enum Cache {
    Redis(RedisCache),
    Kv(std::sync::Arc<KvCache>),
}

impl Cache {
    pub fn from_env() -> Self {
        match env::var("CACHE_BACKEND").as_deref() {
            Ok("kv") => Cache::Kv(std::sync::Arc::new(KvCache::new(
                &env::var("CF_ACCOUNT_ID").expect("CF_ACCOUNT_ID"),
                &env::var("CF_KV_NAMESPACE").expect("CF_KV_NAMESPACE"),
                &env::var("CF_API_TOKEN").expect("CF_API_TOKEN"),
            ))),
            _ => Cache::Redis(RedisCache::new(
                &env::var("REDIS_URL").unwrap_or_else(|_| "redis://localhost:6379".into()),
            )),
        }
    }

    pub fn kind(&self) -> &'static str {
        match self {
            Cache::Redis(_) => "redis",
            Cache::Kv(_) => "kv",
        }
    }

    pub async fn get(&self, key: &str) -> anyhow::Result<Option<String>> {
        match self {
            Cache::Redis(c) => {
                let mut conn = c.client.get_async_connection().await?;
                redis::AsyncCommands::get(&mut conn, key).await.map_err(anyhow::Error::from)
            }
            Cache::Kv(c) => {
                let res = c.client.get(c.url(key)).send().await?;
                if res.status().is_success() {
                    Ok(Some(res.text().await?))
                } else {
                    Ok(None)
                }
            }
        }
    }

    pub async fn set(&self, key: &str, value: &str, ttl_secs: u64) -> anyhow::Result<()> {
        match self {
            Cache::Redis(c) => {
                let mut conn = c.client.get_async_connection().await?;
                redis::AsyncCommands::set_ex(&mut conn, key, value, ttl_secs)
                    .await
                    .map_err(anyhow::Error::from)
            }
            Cache::Kv(c) => {
                c.client
                    .put(c.url(key))
                    .bearer_auth(&c.token)
                    .header("Content-Type", "text/plain")
                    .query(&[("expiration_ttl", ttl_secs.to_string())])
                    .body(value.to_string())
                    .send()
                    .await?;
                Ok(())
            }
        }
    }

    pub async fn delete(&self, key: &str) -> anyhow::Result<()> {
        match self {
            Cache::Redis(c) => {
                let mut conn = c.client.get_async_connection().await?;
                redis::AsyncCommands::del::<_, ()>(&mut conn, key).await.map_err(anyhow::Error::from)
            }
            Cache::Kv(c) => {
                c.client.delete(c.url(key)).bearer_auth(&c.token).send().await?;
                Ok(())
            }
        }
    }
}