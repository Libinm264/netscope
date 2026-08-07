package clickhouse

import (
	"context"
	"log/slog"
)

// Migrate creates (or idempotently alters) every ClickHouse table Nexor
// depends on. It is safe to call on every startup — every statement uses
// `IF NOT EXISTS` / `ADD COLUMN IF NOT EXISTS` or is itself idempotent
// (ReplacingMergeTree seed rows guarded by `WHERE NOT EXISTS`).
//
// Extracted from main.go's runMigrations so integration tests can stand up a
// real schema against a throwaway ClickHouse container (see
// hub/api/testutil) without duplicating ~400 lines of DDL.
//
// Never edit an existing statement below — always append the next `phaseNN`
// entry and let this function apply it idempotently on the next startup.
func Migrate(ctx context.Context, c *Client) error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS alert_rules (
			id               UUID    DEFAULT generateUUIDv4(),
			name             String,
			metric           LowCardinality(String),
			condition        LowCardinality(String),
			threshold        Float64,
			window_minutes   UInt32  DEFAULT 5,
			webhook_url      String  DEFAULT '',
			enabled          UInt8   DEFAULT 1,
			cooldown_minutes UInt32  DEFAULT 15,
			created_at       DateTime64(3, 'UTC') DEFAULT now64()
		) ENGINE = MergeTree()
		ORDER BY (created_at, id)`,

		`CREATE TABLE IF NOT EXISTS alert_events (
			id        UUID DEFAULT generateUUIDv4(),
			rule_id   String,
			rule_name String,
			metric    String,
			value     Float64,
			threshold Float64,
			fired_at  DateTime64(3, 'UTC'),
			delivered UInt8 DEFAULT 0
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMM(fired_at)
		ORDER BY fired_at
		TTL toDateTime(fired_at) + INTERVAL 30 DAY`,

		`CREATE TABLE IF NOT EXISTS flows (
			id          UUID              DEFAULT generateUUIDv4(),
			agent_id    String,
			hostname    LowCardinality(String),
			ts          DateTime64(3, 'UTC'),
			protocol    LowCardinality(String),
			src_ip      String,
			src_port    UInt16,
			dst_ip      String,
			dst_port    UInt16,
			bytes_in    UInt64            DEFAULT 0,
			bytes_out   UInt64            DEFAULT 0,
			duration_ms UInt32            DEFAULT 0,
			info        String            DEFAULT '',
			http_method LowCardinality(String) DEFAULT '',
			http_path   String            DEFAULT '',
			http_status UInt16            DEFAULT 0,
			dns_query   String            DEFAULT '',
			dns_type    LowCardinality(String) DEFAULT ''
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMM(ts)
		ORDER BY (ts, agent_id, protocol)
		TTL toDateTime(ts) + INTERVAL 90 DAY
		SETTINGS index_granularity = 8192`,

		`CREATE TABLE IF NOT EXISTS agents (
			agent_id      String,
			hostname      LowCardinality(String),
			version       String            DEFAULT '',
			interface     String            DEFAULT '',
			last_seen     DateTime64(3, 'UTC'),
			registered_at DateTime64(3, 'UTC') DEFAULT now64()
		) ENGINE = ReplacingMergeTree(last_seen)
		ORDER BY agent_id`,

		// Phase 6: enrollment tokens
		`CREATE TABLE IF NOT EXISTS enrollment_tokens (
			id         String,
			name       String,
			token      String,
			created_at DateTime64(3, 'UTC') DEFAULT now64(),
			expires_at DateTime64(3, 'UTC'),
			used_count UInt32  DEFAULT 0,
			max_uses   UInt32  DEFAULT 0,
			revoked    UInt8   DEFAULT 0
		) ENGINE = ReplacingMergeTree(created_at)
		ORDER BY id`,

		// v0.7: add max_uses cap to existing deployments
		`ALTER TABLE enrollment_tokens ADD COLUMN IF NOT EXISTS max_uses UInt32 DEFAULT 0`,

		// Phase 6: TLS certificate fleet
		`CREATE TABLE IF NOT EXISTS tls_certs (
			fingerprint String,
			cn          String,
			issuer      String  DEFAULT '',
			expiry      String  DEFAULT '',
			expired     UInt8   DEFAULT 0,
			sans        String  DEFAULT '',
			agent_id    String  DEFAULT '',
			hostname    LowCardinality(String) DEFAULT '',
			src_ip      String  DEFAULT '',
			dst_ip      String  DEFAULT '',
			first_seen  DateTime64(3, 'UTC') DEFAULT now64(),
			last_seen   DateTime64(3, 'UTC') DEFAULT now64()
		) ENGINE = ReplacingMergeTree(last_seen)
		ORDER BY fingerprint`,

		// Phase 7: add integration_type column to alert_rules (idempotent)
		`ALTER TABLE alert_rules ADD COLUMN IF NOT EXISTS
		 integration_type LowCardinality(String) DEFAULT 'webhook'`,

		// Phase 8: geo + threat enrichment columns on flows (idempotent)
		`ALTER TABLE flows ADD COLUMN IF NOT EXISTS country_code LowCardinality(String) DEFAULT ''`,
		`ALTER TABLE flows ADD COLUMN IF NOT EXISTS country_name LowCardinality(String) DEFAULT ''`,
		`ALTER TABLE flows ADD COLUMN IF NOT EXISTS as_org       LowCardinality(String) DEFAULT ''`,
		`ALTER TABLE flows ADD COLUMN IF NOT EXISTS threat_score UInt8 DEFAULT 0`,
		`ALTER TABLE flows ADD COLUMN IF NOT EXISTS threat_level LowCardinality(String) DEFAULT ''`,

		// Phase 9: eBPF process attribution columns on flows (idempotent)
		`ALTER TABLE flows ADD COLUMN IF NOT EXISTS process_name LowCardinality(String) DEFAULT ''`,
		`ALTER TABLE flows ADD COLUMN IF NOT EXISTS pid          UInt32 DEFAULT 0`,

		// Phase 6: API tokens (RBAC)
		`CREATE TABLE IF NOT EXISTS api_tokens (
			id         String,
			name       String,
			role       LowCardinality(String) DEFAULT 'viewer',
			token      String,
			created_at DateTime64(3, 'UTC') DEFAULT now64(),
			last_used  DateTime64(3, 'UTC') DEFAULT now64(),
			revoked    UInt8 DEFAULT 0
		) ENGINE = ReplacingMergeTree(last_used)
		ORDER BY id`,

		// Phase 10: new alert_rules columns (idempotent)
		`ALTER TABLE alert_rules ADD COLUMN IF NOT EXISTS webhook_secret String DEFAULT ''`,
		`ALTER TABLE alert_rules ADD COLUMN IF NOT EXISTS email_to String DEFAULT ''`,

		// Phase 11a: agent fleet enrichment (idempotent ALTER TABLE)
		`ALTER TABLE agents ADD COLUMN IF NOT EXISTS os LowCardinality(String) DEFAULT ''`,
		`ALTER TABLE agents ADD COLUMN IF NOT EXISTS capture_mode LowCardinality(String) DEFAULT 'pcap'`,
		`ALTER TABLE agents ADD COLUMN IF NOT EXISTS ebpf_enabled UInt8 DEFAULT 0`,

		// Phase 11b: K8s pod enrichment on flows (idempotent)
		`ALTER TABLE flows ADD COLUMN IF NOT EXISTS pod_name LowCardinality(String) DEFAULT ''`,
		`ALTER TABLE flows ADD COLUMN IF NOT EXISTS k8s_namespace LowCardinality(String) DEFAULT ''`,

		// Phase 11c: Process policies table
		`CREATE TABLE IF NOT EXISTS process_policies (
			id          UUID DEFAULT generateUUIDv4(),
			name        String,
			process_name String,
			action      LowCardinality(String) DEFAULT 'alert',
			dst_ip_cidr String DEFAULT '',
			dst_port    UInt16 DEFAULT 0,
			description String DEFAULT '',
			enabled     UInt8  DEFAULT 1,
			created_at  DateTime64(3, 'UTC') DEFAULT now64()
		) ENGINE = MergeTree() ORDER BY (created_at, id)`,

		// Phase 12a: Multi-tenant organisations table
		`CREATE TABLE IF NOT EXISTS organisations (
			org_id         String,
			name           String,
			slug           LowCardinality(String),
			agent_quota    Int32    DEFAULT 10,
			retention_days Int32    DEFAULT 90,
			plan           LowCardinality(String) DEFAULT 'community',
			created_at     DateTime64(3, 'UTC') DEFAULT now64(),
			version        UInt64   DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY org_id`,

		// Phase 12b: Org members (identity mapping, no credentials stored)
		`CREATE TABLE IF NOT EXISTS org_members (
			user_id      String,
			org_id       LowCardinality(String) DEFAULT 'default',
			email        String,
			display_name String  DEFAULT '',
			role         LowCardinality(String) DEFAULT 'viewer',
			sso_provider LowCardinality(String) DEFAULT '',
			sso_subject  String  DEFAULT '',
			is_active    UInt8   DEFAULT 1,
			created_at   DateTime64(3, 'UTC') DEFAULT now64(),
			last_seen    DateTime64(3, 'UTC') DEFAULT now64(),
			version      UInt64  DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY (org_id, user_id)`,

		// Phase 12c: Teams
		`CREATE TABLE IF NOT EXISTS teams (
			team_id     String,
			org_id      LowCardinality(String) DEFAULT 'default',
			name        String,
			description String  DEFAULT '',
			created_at  DateTime64(3, 'UTC') DEFAULT now64(),
			version     UInt64  DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY (org_id, team_id)`,

		// Phase 12d: Team membership
		`CREATE TABLE IF NOT EXISTS team_members (
			team_id  String,
			user_id  String,
			org_id   LowCardinality(String) DEFAULT 'default',
			added_at DateTime64(3, 'UTC') DEFAULT now64(),
			version  UInt64 DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY (team_id, user_id)`,

		// Phase 12e: SSO provider configurations (no secrets)
		`CREATE TABLE IF NOT EXISTS sso_configs (
			org_id      LowCardinality(String) DEFAULT 'default',
			provider    LowCardinality(String),
			enabled     UInt8   DEFAULT 0,
			entity_id   String  DEFAULT '',
			sso_url     String  DEFAULT '',
			certificate String  DEFAULT '',
			issuer_url  String  DEFAULT '',
			client_id   String  DEFAULT '',
			updated_at  DateTime64(3, 'UTC') DEFAULT now64(),
			version     UInt64  DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY (org_id, provider)`,

		// Phase 12f: seed default organisation (idempotent via ReplacingMergeTree)
		`INSERT INTO organisations (org_id, name, slug, agent_quota, retention_days, plan)
		 SELECT 'default', 'Default Organisation', 'default', 10, 90, 'community'
		 WHERE NOT EXISTS (
		   SELECT 1 FROM organisations WHERE org_id = 'default'
		 )`,

		// Phase 12g: Local credentials (bcrypt password hashes for email/password login)
		`CREATE TABLE IF NOT EXISTS local_credentials (
			user_id       String,
			org_id        LowCardinality(String) DEFAULT 'default',
			password_hash String,
			updated_at    DateTime64(3, 'UTC') DEFAULT now64(),
			version       UInt64 DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY (org_id, user_id)`,

		// Phase 12h: Invite tokens (single-use, 7-day TTL)
		`CREATE TABLE IF NOT EXISTS invite_tokens (
			token      String,
			user_id    String,
			email      String,
			expires_at DateTime64(3, 'UTC'),
			used       UInt8 DEFAULT 0,
			version    UInt64 DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY token`,

		// Phase 12i: Password reset tokens (single-use, 1-hour TTL)
		`CREATE TABLE IF NOT EXISTS password_reset_tokens (
			token      String,
			user_id    String,
			email      String,
			expires_at DateTime64(3, 'UTC'),
			used       UInt8 DEFAULT 0,
			version    UInt64 DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY token`,

		// Phase 11d: Policy violations log
		`CREATE TABLE IF NOT EXISTS policy_violations (
			id          UUID DEFAULT generateUUIDv4(),
			policy_id   String,
			policy_name String,
			process_name String DEFAULT '',
			pid         UInt32 DEFAULT 0,
			src_ip      String DEFAULT '',
			dst_ip      String DEFAULT '',
			dst_port    UInt16 DEFAULT 0,
			protocol    LowCardinality(String) DEFAULT '',
			agent_id    String DEFAULT '',
			hostname    LowCardinality(String) DEFAULT '',
			violated_at DateTime64(3, 'UTC') DEFAULT now64()
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMM(violated_at)
		ORDER BY (violated_at, policy_id)
		TTL toDateTime(violated_at) + INTERVAL 30 DAY`,

		// Phase 9: audit log — every authenticated API call
		`CREATE TABLE IF NOT EXISTS audit_events (
			id         String,
			token_id   String            DEFAULT '',
			role       LowCardinality(String) DEFAULT '',
			method     LowCardinality(String),
			path       String,
			status     UInt16,
			client_ip  String            DEFAULT '',
			latency_ms UInt32            DEFAULT 0,
			ts         DateTime64(3, 'UTC') DEFAULT now64()
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMM(ts)
		ORDER BY (ts, token_id)
		TTL toDateTime(ts) + INTERVAL 90 DAY`,

		// Phase 13: SIEM sink configurations
		`CREATE TABLE IF NOT EXISTS integrations_config (
			sink_type    LowCardinality(String),
			enabled      UInt8            DEFAULT 0,
			config       String           DEFAULT '{}',
			last_shipped DateTime64(3, 'UTC') DEFAULT toDateTime64(0, 3),
			updated_at   DateTime64(3, 'UTC') DEFAULT now64(),
			version      UInt64           DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY sink_type`,

		// Phase 14: OTel trace correlation — trace_id on flows (idempotent)
		`ALTER TABLE flows ADD COLUMN IF NOT EXISTS trace_id String DEFAULT ''`,

		// Phase 15: Saved flow queries (Community: 10 max; Enterprise: unlimited)
		`CREATE TABLE IF NOT EXISTS saved_queries (
			id          String,
			name        String,
			description String  DEFAULT '',
			filters     String  DEFAULT '{}',
			deleted     UInt8   DEFAULT 0,
			created_at  DateTime64(3, 'UTC') DEFAULT now64(),
			updated_at  DateTime64(3, 'UTC') DEFAULT now64(),
			version     UInt64  DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY id`,

		// Phase 16: Sigma detection rules
		`CREATE TABLE IF NOT EXISTS sigma_rules (
			id          String,
			title       String,
			description String  DEFAULT '',
			severity    LowCardinality(String) DEFAULT 'medium',
			tags        String  DEFAULT '[]',
			query       String  DEFAULT '',
			enabled     UInt8   DEFAULT 1,
			builtin     UInt8   DEFAULT 0,
			created_at  DateTime64(3, 'UTC') DEFAULT now64(),
			updated_at  DateTime64(3, 'UTC') DEFAULT now64(),
			version     UInt64  DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY id`,

		// Phase 17: Sigma match events
		`CREATE TABLE IF NOT EXISTS sigma_matches (
			id         UUID    DEFAULT generateUUIDv4(),
			rule_id    String,
			rule_title String,
			severity   LowCardinality(String) DEFAULT 'medium',
			match_data String  DEFAULT '{}',
			fired_at   DateTime64(3, 'UTC') DEFAULT now64()
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMM(fired_at)
		ORDER BY (fired_at, rule_id)
		TTL toDateTime(fired_at) + INTERVAL 30 DAY`,

		// Phase 18: OTel backend URL on organisations (idempotent)
		`ALTER TABLE organisations ADD COLUMN IF NOT EXISTS otel_backend_url String DEFAULT ''`,

		// Phase 19: Cluster label on agents (idempotent)
		`ALTER TABLE agents ADD COLUMN IF NOT EXISTS cluster LowCardinality(String) DEFAULT ''`,

		// Phase 20: Long-term storage config (Enterprise S3/GCS export)
		`CREATE TABLE IF NOT EXISTS storage_config (
			provider    LowCardinality(String) DEFAULT 's3',
			enabled     UInt8            DEFAULT 0,
			bucket      String           DEFAULT '',
			region      String           DEFAULT '',
			endpoint    String           DEFAULT '',
			access_key  String           DEFAULT '',
			secret_key  String           DEFAULT '',
			prefix      String           DEFAULT 'nexor/flows',
			schedule    LowCardinality(String) DEFAULT 'hourly',
			last_export DateTime64(3, 'UTC') DEFAULT toDateTime64(0, 3),
			updated_at  DateTime64(3, 'UTC') DEFAULT now64(),
			version     UInt64           DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY provider`,

		// Phase 21: Storage export audit log
		`CREATE TABLE IF NOT EXISTS storage_exports (
			id          UUID    DEFAULT generateUUIDv4(),
			window      String,
			object_key  String  DEFAULT '',
			row_count   UInt64  DEFAULT 0,
			exported_at DateTime64(3, 'UTC') DEFAULT now64(),
			error       String  DEFAULT ''
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMM(exported_at)
		ORDER BY exported_at
		TTL toDateTime(exported_at) + INTERVAL 90 DAY`,

		// Phase 17b: Seed the 5 built-in Community detection rules (idempotent)
		`INSERT INTO sigma_rules (id, title, description, severity, tags, query, enabled, builtin, created_at, updated_at, version)
		 SELECT 'builtin-001', 'Port Scan Detection',
		   'Detects hosts probing more than 50 unique destination ports within a 5-minute window.',
		   'high', '["recon","portscan","attack.discovery"]',
		   'SELECT src_ip, count(DISTINCT dst_port) AS port_count, min(ts) AS first_seen FROM flows WHERE ts > now() - INTERVAL 5 MINUTE AND protocol IN (''TCP'',''UDP'') GROUP BY src_ip HAVING port_count > 50',
		   1, 1, now64(), now64(), 1
		 WHERE NOT EXISTS (SELECT 1 FROM sigma_rules WHERE id = 'builtin-001')`,

		`INSERT INTO sigma_rules (id, title, description, severity, tags, query, enabled, builtin, created_at, updated_at, version)
		 SELECT 'builtin-002', 'DNS Tunneling Indicator',
		   'Identifies unusually long DNS query names (>60 chars) indicating DNS-based exfiltration.',
		   'medium', '["dns","exfiltration","attack.c2"]',
		   'SELECT src_ip, hostname, dns_query, ts FROM flows WHERE ts > now() - INTERVAL 10 MINUTE AND protocol = ''DNS'' AND length(dns_query) > 60',
		   1, 1, now64(), now64(), 1
		 WHERE NOT EXISTS (SELECT 1 FROM sigma_rules WHERE id = 'builtin-002')`,

		`INSERT INTO sigma_rules (id, title, description, severity, tags, query, enabled, builtin, created_at, updated_at, version)
		 SELECT 'builtin-003', 'Cleartext Credential Submission',
		   'Detects HTTP POST to authentication paths over plain HTTP risking credential exposure.',
		   'high', '["credentials","http","attack.credential_access"]',
		   'SELECT src_ip, dst_ip, dst_port, http_path, hostname, ts FROM flows WHERE ts > now() - INTERVAL 15 MINUTE AND protocol = ''HTTP'' AND http_method = ''POST'' AND (http_path LIKE ''%/login%'' OR http_path LIKE ''%/auth%'' OR http_path LIKE ''%/signin%'') AND dst_port != 443',
		   1, 1, now64(), now64(), 1
		 WHERE NOT EXISTS (SELECT 1 FROM sigma_rules WHERE id = 'builtin-003')`,

		`INSERT INTO sigma_rules (id, title, description, severity, tags, query, enabled, builtin, created_at, updated_at, version)
		 SELECT 'builtin-004', 'Unexpected Outbound High Port',
		   'Flags large outbound connections to ephemeral ports that may indicate beaconing or reverse shells.',
		   'medium', '["c2","beaconing","attack.command_and_control"]',
		   'SELECT src_ip, dst_ip, dst_port, protocol, bytes_out, hostname, ts FROM flows WHERE ts > now() - INTERVAL 5 MINUTE AND dst_port > 49151 AND protocol = ''TCP'' AND bytes_out > 10000',
		   1, 1, now64(), now64(), 1
		 WHERE NOT EXISTS (SELECT 1 FROM sigma_rules WHERE id = 'builtin-004')`,

		`INSERT INTO sigma_rules (id, title, description, severity, tags, query, enabled, builtin, created_at, updated_at, version)
		 SELECT 'builtin-005', 'Privileged Process Network Activity',
		   'Detects shell or interpreter processes making unexpected outbound connections post-exploitation indicator.',
		   'critical', '["process","shell","attack.execution","attack.lateral_movement"]',
		   'SELECT process_name, pid, src_ip, dst_ip, dst_port, hostname, ts FROM flows WHERE ts > now() - INTERVAL 5 MINUTE AND protocol = ''TCP'' AND process_name IN (''bash'',''sh'',''zsh'',''python'',''python3'',''powershell'',''pwsh'',''cmd'',''perl'',''ruby'') AND dst_port NOT IN (22, 80, 443)',
		   1, 1, now64(), now64(), 1
		 WHERE NOT EXISTS (SELECT 1 FROM sigma_rules WHERE id = 'builtin-005')`,

		// ── v0.5 Fleet Intelligence ───────────────────────────────────────────

		// Phase 22: Cloud flow sources (VPC Flow Logs config per cloud account)
		`CREATE TABLE IF NOT EXISTS cloud_flow_sources (
			id          String,
			provider    LowCardinality(String) DEFAULT 'aws',
			name        String                 DEFAULT '',
			config      String                 DEFAULT '{}',
			enabled     UInt8                  DEFAULT 1,
			last_pulled DateTime64(3,'UTC')    DEFAULT toDateTime64(0,3),
			error_msg   String                 DEFAULT '',
			created_at  DateTime64(3,'UTC')    DEFAULT now64(),
			version     UInt64                 DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY id`,

		// Phase 23: Cloud pull audit log
		`CREATE TABLE IF NOT EXISTS cloud_flow_pull_log (
			id            UUID    DEFAULT generateUUIDv4(),
			source_id     String,
			provider      LowCardinality(String) DEFAULT 'aws',
			rows_ingested UInt64  DEFAULT 0,
			pulled_at     DateTime64(3,'UTC') DEFAULT now64(),
			duration_ms   UInt32  DEFAULT 0,
			error         String  DEFAULT ''
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMM(pulled_at)
		ORDER BY pulled_at
		TTL toDateTime(pulled_at) + INTERVAL 30 DAY`,

		// Phase 24: Source label on flows (agent vs cloud-pull)
		`ALTER TABLE flows ADD COLUMN IF NOT EXISTS source LowCardinality(String) DEFAULT 'agent'`,

		// Phase 25: Agent remote-config push table
		`CREATE TABLE IF NOT EXISTS agent_configs (
			agent_id   String,
			config     String              DEFAULT '{}',
			pushed_at  DateTime64(3,'UTC') DEFAULT now64(),
			ack_at     DateTime64(3,'UTC') DEFAULT toDateTime64(0,3),
			version    UInt64              DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY agent_id`,

		// Phase 26: Config version on agents (agent reports running config)
		`ALTER TABLE agents ADD COLUMN IF NOT EXISTS config_version String DEFAULT ''`,

		// Phase 27: Compliance report schedules (Enterprise)
		`CREATE TABLE IF NOT EXISTS compliance_report_schedules (
			id         String,
			name       String                 DEFAULT '',
			framework  LowCardinality(String) DEFAULT 'soc2',
			format     LowCardinality(String) DEFAULT 'pdf',
			schedule   LowCardinality(String) DEFAULT 'weekly',
			recipients String                 DEFAULT '[]',
			enabled    UInt8                  DEFAULT 1,
			last_sent  DateTime64(3,'UTC')    DEFAULT toDateTime64(0,3),
			created_at DateTime64(3,'UTC')    DEFAULT now64(),
			version    UInt64                 DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY id`,

		// Phase 28: Compliance report run history
		`CREATE TABLE IF NOT EXISTS compliance_report_runs (
			id          UUID    DEFAULT generateUUIDv4(),
			schedule_id String,
			framework   LowCardinality(String) DEFAULT 'soc2',
			format      LowCardinality(String) DEFAULT 'pdf',
			recipients  String  DEFAULT '[]',
			rows        UInt64  DEFAULT 0,
			sent_at     DateTime64(3,'UTC') DEFAULT now64(),
			error       String  DEFAULT ''
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMM(sent_at)
		ORDER BY sent_at
		TTL toDateTime(sent_at) + INTERVAL 90 DAY`,

		// Phase 29: In-hub incident timeline (Enterprise)
		`CREATE TABLE IF NOT EXISTS incidents (
			id           String,
			title        String                 DEFAULT '',
			severity     LowCardinality(String) DEFAULT 'medium',
			status       LowCardinality(String) DEFAULT 'open',
			source       LowCardinality(String) DEFAULT 'sigma',
			source_id    String                 DEFAULT '',
			notes        String                 DEFAULT '',
			external_ref String                 DEFAULT '',
			created_at   DateTime64(3,'UTC')    DEFAULT now64(),
			updated_at   DateTime64(3,'UTC')    DEFAULT now64(),
			version      UInt64                 DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY (created_at, id)`,

		// Phase 31 (v0.6): Traffic baselines — 7-day rolling mean/stddev
		`CREATE TABLE IF NOT EXISTS traffic_baselines (
			agent_id        String,
			protocol        LowCardinality(String),
			hour_of_week    UInt8,
			flow_count_mean  Float64 DEFAULT 0,
			flow_count_std   Float64 DEFAULT 0,
			bytes_in_mean    Float64 DEFAULT 0,
			bytes_out_mean   Float64 DEFAULT 0,
			sample_count     UInt32  DEFAULT 0,
			computed_at      DateTime64(3, 'UTC') DEFAULT now64(),
			version          UInt64  DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY (agent_id, protocol, hour_of_week)`,

		// Phase 32 (v0.6): Anomaly events — Z-score outliers
		`CREATE TABLE IF NOT EXISTS anomaly_events (
			id           UUID    DEFAULT generateUUIDv4(),
			agent_id     String,
			hostname     LowCardinality(String) DEFAULT '',
			protocol     LowCardinality(String),
			anomaly_type LowCardinality(String) DEFAULT 'spike',
			z_score      Float64 DEFAULT 0,
			observed     Float64 DEFAULT 0,
			expected     Float64 DEFAULT 0,
			description  String  DEFAULT '',
			severity     LowCardinality(String) DEFAULT 'low',
			detected_at  DateTime64(3, 'UTC') DEFAULT now64()
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMM(detected_at)
		ORDER BY (detected_at, agent_id)
		TTL toDateTime(detected_at) + INTERVAL 30 DAY`,

		// G4: Custom dashboards
		`CREATE TABLE IF NOT EXISTS dashboards (
			id          String,
			name        String,
			description String  DEFAULT '',
			widgets     String  DEFAULT '[]',
			is_deleted  UInt8   DEFAULT 0,
			created_at  DateTime64(3, 'UTC') DEFAULT now64(),
			updated_at  DateTime64(3, 'UTC') DEFAULT now64(),
			version     UInt64  DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY id`,

		// Phase 30: Incident workflow config (per-integration credentials)
		`CREATE TABLE IF NOT EXISTS incident_workflow_config (
			integration LowCardinality(String) DEFAULT 'pagerduty',
			enabled     UInt8                  DEFAULT 0,
			config      String                 DEFAULT '{}',
			updated_at  DateTime64(3,'UTC')    DEFAULT now64(),
			version     UInt64                 DEFAULT 1
		) ENGINE = ReplacingMergeTree(version)
		ORDER BY integration`,

		// Phase 31: Agent performance telemetry (v0.7)
		`CREATE TABLE IF NOT EXISTS agent_perf (
			agent_id        String,
			ts              DateTime64(3, 'UTC') DEFAULT now64(),
			cpu_pct         Float32  DEFAULT 0,
			mem_mb          UInt64   DEFAULT 0,
			packets_dropped UInt64   DEFAULT 0
		) ENGINE = MergeTree()
		ORDER BY (agent_id, ts)
		TTL ts + INTERVAL 30 DAY`,
	}

	for _, q := range ddl {
		if err := c.Exec(ctx, q); err != nil {
			return err
		}
	}

	slog.Info("schema migrations complete")
	return nil
}
