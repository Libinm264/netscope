/// SQLite session persistence using sqlx.
/// Saves and loads flows to/from .nscope files.
use crate::dto::FlowDto;
use anyhow::Result;
use sqlx::{sqlite::SqliteConnectOptions, SqlitePool};
use std::str::FromStr;

pub async fn open_or_create(path: &str) -> Result<SqlitePool> {
    let opts = SqliteConnectOptions::from_str(&format!("sqlite:{}", path))?
        .create_if_missing(true);
    let pool = SqlitePool::connect_with(opts).await?;
    migrate(&pool).await?;
    Ok(pool)
}

async fn migrate(pool: &SqlitePool) -> Result<()> {
    sqlx::query(
        r#"
        CREATE TABLE IF NOT EXISTS sessions (
            id   TEXT PRIMARY KEY,
            name TEXT NOT NULL,
            created_at TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS flows (
            id           TEXT PRIMARY KEY,
            session_id   TEXT NOT NULL REFERENCES sessions(id),
            timestamp    TEXT NOT NULL,
            time_str     TEXT NOT NULL,
            src_ip       TEXT NOT NULL,
            dst_ip       TEXT NOT NULL,
            src_port     INTEGER NOT NULL,
            dst_port     INTEGER NOT NULL,
            protocol     TEXT NOT NULL,
            length       INTEGER NOT NULL,
            info         TEXT NOT NULL,
            payload_json TEXT
        );
        "#,
    )
    .execute(pool)
    .await?;
    Ok(())
}

pub async fn save_flows(pool: &SqlitePool, session_name: &str, flows: &[FlowDto]) -> Result<()> {
    let session_id = uuid_v4();
    let now = chrono::Utc::now().to_rfc3339();

    sqlx::query(
        "INSERT OR REPLACE INTO sessions (id, name, created_at) VALUES (?, ?, ?)",
    )
    .bind(&session_id)
    .bind(session_name)
    .bind(&now)
    .execute(pool)
    .await?;

    let mut tx = pool.begin().await?;
    for flow in flows {
        let payload_json = serde_json::to_string(&flow).ok();
        sqlx::query(
            r#"INSERT OR REPLACE INTO flows
               (id, session_id, timestamp, time_str, src_ip, dst_ip,
                src_port, dst_port, protocol, length, info, payload_json)
               VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"#,
        )
        .bind(&flow.id)
        .bind(&session_id)
        .bind(flow.timestamp.to_rfc3339())
        .bind(&flow.time_str)
        .bind(&flow.src_ip)
        .bind(&flow.dst_ip)
        .bind(flow.src_port as i64)
        .bind(flow.dst_port as i64)
        .bind(&flow.protocol)
        .bind(flow.length as i64)
        .bind(&flow.info)
        .bind(payload_json)
        .execute(&mut *tx)
        .await?;
    }
    tx.commit().await?;
    Ok(())
}

pub async fn load_flows(pool: &SqlitePool) -> Result<Vec<FlowDto>> {
    let rows = sqlx::query_as::<_, (String,)>(
        "SELECT payload_json FROM flows ORDER BY timestamp ASC",
    )
    .fetch_all(pool)
    .await?;

    let flows = rows
        .into_iter()
        .filter_map(|(json,)| serde_json::from_str(&json).ok())
        .collect();

    Ok(flows)
}

fn uuid_v4() -> String {
    use std::time::{SystemTime, UNIX_EPOCH};
    let t = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos();
    format!("{:032x}", t)
}
