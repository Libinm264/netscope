mod hub_client;

use anyhow::Result;
use capture::{list_interfaces, start_capture, CaptureError};
use clap::{Parser, Subcommand};
use config::{AgentConfig, OutputMode};
use hub_client::HubClient;
use parser::session::SessionManager;
use proto::FlowPayload;
use std::sync::mpsc;
use std::thread;
use tracing::{error, info, warn};
use tracing_subscriber::EnvFilter;

#[cfg(all(target_os = "linux", feature = "ebpf"))]
use chrono::{DateTime, Utc};
#[cfg(all(target_os = "linux", feature = "ebpf"))]
use proto::ProcessInfo;
#[cfg(all(target_os = "linux", feature = "ebpf"))]
use uuid::Uuid;
#[cfg(all(target_os = "linux", feature = "ebpf"))]
use ebpf_loader::{start as ebpf_start, EbpfConfig, EbpfEvent, SslDirection};

#[derive(Parser)]
#[command(
    name = "netscope-agent",
    version = "0.1.0",
    about = "NetScope network observability agent — captures and decodes network traffic"
)]
struct Cli {
    #[command(subcommand)]
    command: Command,
}

#[derive(Subcommand)]
enum Command {
    /// Capture live traffic on a network interface and decode HTTP/DNS flows
    Capture {
        /// Network interface to capture on (e.g. en0, eth0, lo)
        #[arg(short, long, default_value = "en0")]
        interface: String,

        /// BPF filter expression (e.g. "tcp port 80")
        #[arg(short = 'f', long)]
        filter: Option<String>,

        /// Output destination: stdout or hub
        #[arg(short, long, default_value = "stdout")]
        output: String,

        /// Hub WebSocket URL (required when output=hub)
        #[arg(long)]
        hub_url: Option<String>,

        /// Agent API key for Hub authentication
        #[arg(long)]
        api_key: Option<String>,
    },

    /// List available network interfaces on this machine
    ListInterfaces,

    /// Run the eBPF-based capture engine (Linux ≥ 5.8, requires CAP_BPF).
    ///
    /// Unlike pcap-mode, eBPF capture intercepts plaintext at the SSL layer
    /// before encryption and provides per-process attribution for every flow.
    ///
    /// Build first: `cargo xtask build-ebpf --release`
    /// Run with:    `sudo netscope-agent ebpf --hub-url … --api-key …`
    #[cfg(target_os = "linux")]
    Ebpf {
        /// Hub API base URL (e.g. https://hub.example.com)
        #[arg(long)]
        hub_url: Option<String>,
        /// Agent API key
        #[arg(long)]
        api_key: Option<String>,
        /// Override libssl path (auto-detected if not set)
        #[arg(long)]
        libssl_path: Option<String>,
    },
}

fn main() {
    // Initialise tracing — default to INFO, override with RUST_LOG
    tracing_subscriber::fmt()
        .with_env_filter(
            EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("info")),
        )
        .init();

    let cli = Cli::parse();

    if let Err(e) = run(cli) {
        error!("{:#}", e);
        std::process::exit(1);
    }
}

fn run(cli: Cli) -> Result<()> {
    match cli.command {
        Command::ListInterfaces => {
            let ifaces = list_interfaces().map_err(|e| anyhow::anyhow!("{}", e))?;
            if ifaces.is_empty() {
                println!("No network interfaces found.");
            } else {
                println!("Available network interfaces:");
                for iface in &ifaces {
                    println!("  {}", iface);
                }
            }
        }

        #[cfg(target_os = "linux")]
        Command::Ebpf { hub_url, api_key, libssl_path } => {
            run_ebpf(hub_url, api_key, libssl_path)?;
        }

        Command::Capture {
            interface,
            filter,
            output,
            hub_url,
            api_key,
        } => {
            let output_mode = match output.as_str() {
                "stdout" => OutputMode::Stdout,
                "hub" => OutputMode::Hub,
                other => anyhow::bail!("Unknown output mode '{}'. Use 'stdout' or 'hub'.", other),
            };

            let cfg = AgentConfig {
                interface: interface.clone(),
                bpf_filter: filter,
                output: output_mode,
                hub_url,
                api_key,
                ..Default::default()
            };

            info!("Starting NetScope Agent on interface '{}'", interface);
            info!("Press Ctrl+C to stop.");

            run_capture(cfg)?;
        }
    }

    Ok(())
}

fn run_capture(cfg: AgentConfig) -> Result<()> {
    let (tx, rx) = mpsc::channel();

    let cfg_clone = cfg.clone();

    // Spawn packet capture on a dedicated OS thread (libpcap is blocking)
    let capture_thread = thread::spawn(move || {
        if let Err(e) = start_capture(&cfg_clone, tx) {
            match e {
                CaptureError::InsufficientPrivileges => {
                    eprintln!();
                    eprintln!("ERROR: Packet capture requires elevated privileges.");
                    eprintln!();
                    eprintln!("  macOS / Linux:  sudo netscope-agent capture --interface {}", cfg_clone.interface);
                    eprintln!("  Linux (no sudo): sudo setcap cap_net_raw+eip $(which netscope-agent)");
                    eprintln!();
                    std::process::exit(1);
                }
                CaptureError::InterfaceNotFound(ref iface) => {
                    eprintln!();
                    eprintln!("ERROR: Interface '{}' not found.", iface);
                    eprintln!("Run 'netscope-agent list-interfaces' to see available interfaces.");
                    eprintln!();
                    std::process::exit(1);
                }
                other => {
                    eprintln!("Capture error: {}", other);
                    std::process::exit(1);
                }
            }
        }
    });

    // Build hub client if hub output is requested
    let hub_output = matches!(cfg.output, OutputMode::Hub);
    let mut hub: Option<HubClient> = if hub_output {
        match (cfg.hub_url.as_ref(), cfg.api_key.as_ref()) {
            (Some(url), Some(key)) => {
                match HubClient::new(url, key) {
                    Ok(c) => {
                        info!(
                            agent_id = c.agent_id(),
                            hostname = c.hostname(),
                            hub_url = %url,
                            "Hub client initialised"
                        );
                        Some(c)
                    }
                    Err(e) => {
                        warn!("Failed to create hub client: {:#} — falling back to stdout", e);
                        None
                    }
                }
            }
            _ => {
                warn!("--hub-url and --api-key are required for hub output — falling back to stdout");
                None
            }
        }
    } else {
        None
    };

    let mut session_mgr = SessionManager::new();
    let mut flow_count = 0u64;

    // Main loop: receive PacketEvents and decode them into flows
    for packet_event in &rx {
        let flows = session_mgr.process(&packet_event);

        for flow in flows {
            flow_count += 1;

            if let Some(ref mut client) = hub {
                if let Err(e) = client.send_flow(&flow) {
                    warn!("Hub send error: {:#}", e);
                    // Fall back to printing so we don't lose the flow
                    print_flow(&flow, flow_count);
                }
            } else {
                print_flow(&flow, flow_count);
            }
        }
    }

    // Flush any remaining buffered flows before exiting
    if let Some(ref mut client) = hub {
        if let Err(e) = client.flush() {
            warn!("Hub flush on exit failed: {:#}", e);
        }
    }

    capture_thread.join().ok();
    Ok(())
}

// ── eBPF entry point ──────────────────────────────────────────────────────────

#[cfg(target_os = "linux")]
fn run_ebpf(
    hub_url: Option<String>,
    api_key: Option<String>,
    libssl_path: Option<String>,
) -> Result<()> {
    #[cfg(not(feature = "ebpf"))]
    {
        anyhow::bail!(
            "eBPF support is not compiled in.\n\
             Rebuild with: cargo build --features ebpf\n\
             Then compile BPF programs: cargo xtask build-ebpf --release"
        );
    }

    #[cfg(feature = "ebpf")]
    {
        // Spin up a dedicated hub-sender thread that owns the blocking HubClient.
        // The async eBPF loop converts events to flows and enqueues via this channel,
        // avoiding calling reqwest::blocking inside an async runtime context.
        let hub_tx: Option<mpsc::SyncSender<proto::Flow>> =
            match (hub_url.as_ref(), api_key.as_ref()) {
                (Some(url), Some(key)) => match HubClient::new(url, key) {
                    Ok(mut client) => {
                        let (tx, rx) = mpsc::sync_channel::<proto::Flow>(512);
                        info!(
                            agent_id = client.agent_id(),
                            hostname = client.hostname(),
                            hub_url = %url,
                            "Hub client ready (eBPF mode)"
                        );
                        thread::spawn(move || {
                            for flow in rx {
                                if let Err(e) = client.send_flow(&flow) {
                                    warn!("Hub send error: {:#}", e);
                                }
                            }
                            // Channel closed — flush any remaining buffered flows
                            if let Err(e) = client.flush() {
                                warn!("Hub final flush: {:#}", e);
                            }
                        });
                        Some(tx)
                    }
                    Err(e) => {
                        warn!("Hub client failed: {:#} — printing to stdout", e);
                        None
                    }
                },
                _ => {
                    warn!("--hub-url / --api-key not set — printing events to stdout");
                    None
                }
            };

        let rt = tokio::runtime::Runtime::new()?;
        rt.block_on(async move {
            let cfg = EbpfConfig {
                libssl_path,
                channel_capacity: 8192,
            };

            info!("Starting eBPF capture engine (SSL intercept + TCP attribution)");
            info!("Flows will include per-process attribution (PID + process name)");

            let mut rx = ebpf_start(cfg).await?;
            let mut count = 0u64;

            while let Some(event) = rx.recv().await {
                let flow_opt: Option<proto::Flow> = match &event {
                    EbpfEvent::Ssl(e) => {
                        // Convert SSL plaintext event → Flow.
                        // Both Read (inbound) and Write (outbound) carry useful data;
                        // we parse application-layer protocol from either direction.
                        Some(ssl_event_to_flow(e))
                    }
                    EbpfEvent::TcpConnect(e) if e.success => {
                        // Successful TCP connect → lightweight connection-tracking flow
                        Some(tcp_connect_to_flow(e))
                    }
                    EbpfEvent::TcpConnect(_) => None, // failed connect, skip
                };

                if let Some(flow) = flow_opt {
                    count += 1;
                    print_flow(&flow, count);

                    if let Some(ref tx) = hub_tx {
                        // try_send: non-blocking; drop flow if channel is full rather
                        // than blocking the async event loop.
                        if tx.try_send(flow).is_err() {
                            warn!("Hub channel full — dropping flow #{}", count);
                        }
                    }
                }
            }

            Ok::<_, anyhow::Error>(())
        })?;
        Ok(())
    }
}

// ── eBPF → Flow conversion helpers ────────────────────────────────────────────

/// Parse raw plaintext (from an SSL uprobe) into an HTTP FlowPayload.
/// Returns None if the data doesn't look like HTTP/1.x.
#[cfg(all(target_os = "linux", feature = "ebpf"))]
fn parse_http_plaintext(data: &str, now: DateTime<Utc>) -> Option<proto::FlowPayload> {
    let first_line = data.lines().next()?.trim();

    // ── HTTP response: "HTTP/1.x NNN Status Text" ────────────────────────────
    if first_line.starts_with("HTTP/1.") {
        let mut parts = first_line.splitn(3, ' ');
        let version     = parts.next()?.to_string();
        let status_code: u16 = parts.next()?.parse().ok()?;
        let status_text = parts.next().unwrap_or("").to_string();

        let (headers, body_preview) = parse_headers_body(data);
        return Some(proto::FlowPayload::Http(proto::HttpFlow {
            request: None,
            response: Some(proto::HttpResponse {
                status_code,
                status_text,
                version,
                headers,
                body_preview,
                timestamp: now,
            }),
            latency_ms: None,
        }));
    }

    // ── HTTP request: "METHOD path HTTP/1.x" ─────────────────────────────────
    const METHODS: &[&str] = &[
        "GET ", "POST ", "PUT ", "DELETE ", "PATCH ",
        "HEAD ", "OPTIONS ", "CONNECT ", "TRACE ",
    ];
    if METHODS.iter().any(|m| first_line.starts_with(m)) {
        let mut parts = first_line.splitn(3, ' ');
        let method  = parts.next()?.to_string();
        let path    = parts.next()?.to_string();
        let version = parts.next().unwrap_or("HTTP/1.1").to_string();

        let (headers, body_preview) = parse_headers_body(data);
        return Some(proto::FlowPayload::Http(proto::HttpFlow {
            request: Some(proto::HttpRequest {
                method,
                path,
                version,
                headers,
                body_preview,
                timestamp: now,
            }),
            response: None,
            latency_ms: None,
        }));
    }

    None
}

/// Parse header lines and optional body preview from raw HTTP text.
/// Skips the first (request/status) line.
#[cfg(all(target_os = "linux", feature = "ebpf"))]
fn parse_headers_body(raw: &str) -> (Vec<(String, String)>, Option<String>) {
    let mut headers = Vec::new();
    let mut body_lines: Vec<&str> = Vec::new();
    let mut in_body = false;

    for line in raw.lines().skip(1) {
        if in_body {
            body_lines.push(line);
            if body_lines.len() >= 5 {
                break;
            }
        } else if line.is_empty() {
            in_body = true;
        } else if let Some((k, v)) = line.split_once(": ") {
            headers.push((k.to_string(), v.to_string()));
        }
    }

    let body_preview = if body_lines.is_empty() {
        None
    } else {
        Some(body_lines.join("\n").chars().take(512).collect())
    };

    (headers, body_preview)
}

/// Convert a captured SSL plaintext event into a proto::Flow.
#[cfg(all(target_os = "linux", feature = "ebpf"))]
fn ssl_event_to_flow(e: &ebpf_loader::SslFlowEvent) -> proto::Flow {
    let payload = parse_http_plaintext(&e.data, e.timestamp);

    // Bytes direction: Write = we're sending data out, Read = receiving
    let (bytes_out, bytes_in) = match e.direction {
        SslDirection::Write => (e.data.len() as u64, 0u64),
        SslDirection::Read  => (0u64, e.data.len() as u64),
    };

    // Protocol: HTTPS for well-known TLS ports; otherwise mark as TLS
    let protocol = match e.dst_port {
        443 | 8443 | 4433 => proto::Protocol::Https,
        _ if payload.is_some() => proto::Protocol::Https,
        _ => proto::Protocol::Tls,
    };

    proto::Flow {
        id:        Uuid::new_v4().to_string(),
        timestamp: e.timestamp,
        src_ip:    e.src_ip.to_string(),
        dst_ip:    e.dst_ip.to_string(),
        src_port:  e.src_port,
        dst_port:  e.dst_port,
        protocol,
        bytes_in,
        bytes_out,
        payload,
        tcp_stats: None,
        process: Some(ProcessInfo {
            pid:  e.pid,
            name: e.comm.clone(),
        }),
    }
}

/// Convert a TCP connection event into a lightweight proto::Flow
/// (no application-layer payload, just connection attribution).
#[cfg(all(target_os = "linux", feature = "ebpf"))]
fn tcp_connect_to_flow(e: &ebpf_loader::TcpFlowEvent) -> proto::Flow {
    proto::Flow {
        id:        Uuid::new_v4().to_string(),
        timestamp: e.timestamp,
        src_ip:    e.src_ip.to_string(),
        dst_ip:    e.dst_ip.to_string(),
        src_port:  e.src_port,
        dst_port:  e.dst_port,
        protocol:  proto::Protocol::Tcp,
        bytes_in:  0,
        bytes_out: 0,
        payload:   None,
        tcp_stats: None,
        process: Some(ProcessInfo {
            pid:  e.pid,
            name: e.comm.clone(),
        }),
    }
}

// ── Terminal output ────────────────────────────────────────────────────────────

fn print_flow(flow: &proto::Flow, n: u64) {
    let ts = flow.timestamp.format("%H:%M:%S%.3f");

    // eBPF flows carry process attribution — show it as a dim prefix
    let proc_prefix = flow.process.as_ref()
        .map(|p| format!("\x1b[2m[{}:{}]\x1b[0m ", p.name, p.pid))
        .unwrap_or_default();

    match &flow.payload {
        Some(FlowPayload::Http(http)) => {
            let req = http.request.as_ref();
            let resp = http.response.as_ref();

            let method = req.map(|r| r.method.as_str()).unwrap_or("-");
            let path = req.map(|r| r.path.as_str()).unwrap_or("-");
            let host = req
                .and_then(|r| r.headers.iter().find(|(k, _)| k.eq_ignore_ascii_case("host")))
                .map(|(_, v)| v.as_str())
                .unwrap_or(&flow.dst_ip);
            let status = resp
                .map(|r| r.status_code.to_string())
                .unwrap_or_else(|| "-".to_string());
            let latency = http
                .latency_ms
                .map(|ms| format!("{}ms", ms))
                .unwrap_or_else(|| "-".to_string());

            println!(
                "[{}] {}\x1b[34mHTTP\x1b[0m #{} {} {} {} → {} ({}) {}",
                ts, proc_prefix, n, method, host, path, status, latency,
                format!("{}:{} → {}:{}", flow.src_ip, flow.src_port, flow.dst_ip, flow.dst_port)
            );

            if let Some(req) = req {
                if std::env::var("NETSCOPE_VERBOSE").is_ok() {
                    for (k, v) in &req.headers {
                        println!("      > {}: {}", k, v);
                    }
                    if let Some(body) = &req.body_preview {
                        println!("      > body: {}", body);
                    }
                }
            }
        }

        Some(FlowPayload::Dns(dns)) => {
            let direction = if dns.is_response { "RESP" } else { "QUERY" };
            let answers: Vec<String> = dns
                .answers
                .iter()
                .map(|a| format!("{} {} {}", a.record_type, a.name, a.data))
                .collect();

            if dns.is_response && !dns.answers.is_empty() {
                println!(
                    "[{}] {}\x1b[35mDNS\x1b[0m #{} {} {} {} → [{}]",
                    ts, proc_prefix, n, direction, dns.query_name, dns.query_type,
                    answers.join(", ")
                );
            } else if !dns.is_response {
                println!(
                    "[{}] {}\x1b[35mDNS\x1b[0m #{} {} {} {}",
                    ts, proc_prefix, n, direction, dns.query_name, dns.query_type
                );
            }
        }

        Some(FlowPayload::Tls(tls)) => {
            let detail = match tls.record_type.as_str() {
                "ClientHello" => format!(
                    "{}{}",
                    tls.sni.as_deref().unwrap_or("?"),
                    if tls.has_weak_cipher { " [weak cipher]" } else { "" }
                ),
                "ServerHello" => tls.chosen_cipher.clone().unwrap_or_default(),
                "Certificate" => format!(
                    "{}{}",
                    tls.cert_cn.as_deref().unwrap_or("?"),
                    if tls.cert_expired { " [EXPIRED]" } else { "" }
                ),
                "Alert" => format!(
                    "{} {}",
                    tls.alert_level.as_deref().unwrap_or(""),
                    tls.alert_description.as_deref().unwrap_or("")
                ),
                other => other.to_string(),
            };
            println!(
                "[{}] {}\x1b[36mTLS\x1b[0m #{} {} {} {}:{} → {}:{}",
                ts, proc_prefix, n, tls.record_type, detail,
                flow.src_ip, flow.src_port, flow.dst_ip, flow.dst_port
            );
        }

        Some(FlowPayload::Icmp(icmp)) => {
            let rtt = icmp
                .rtt_ms
                .map(|r| format!(" ({:.1}ms)", r))
                .unwrap_or_default();
            println!(
                "[{}] {}\x1b[36mICMP\x1b[0m #{} {}{} {} → {}",
                ts, proc_prefix, n, icmp.type_str, rtt, flow.src_ip, flow.dst_ip
            );
        }

        Some(FlowPayload::Arp(arp)) => {
            println!(
                "[{}] {}\x1b[33mARP\x1b[0m  #{} {} {} → {} ({})",
                ts, proc_prefix, n, arp.operation, arp.sender_ip, arp.target_ip, arp.sender_mac
            );
        }

        Some(FlowPayload::Http2(h2)) => {
            let method  = h2.request.as_ref().map(|r| r.method.as_str()).unwrap_or("-");
            let path    = h2.request.as_ref().map(|r| r.path.as_str()).unwrap_or("-");
            let status  = h2.response.as_ref().map(|r| r.status_code.to_string())
                           .unwrap_or_else(|| "-".to_string());
            let latency = h2.latency_ms.map(|ms| format!("{}ms", ms))
                           .unwrap_or_else(|| "-".to_string());
            let addr = format!("{}:{} → {}:{}", flow.src_ip, flow.src_port, flow.dst_ip, flow.dst_port);
            match (&h2.grpc_service, &h2.grpc_method) {
                (Some(svc), Some(meth)) => println!(
                    "[{}] {}\x1b[34mgRPC\x1b[0m   #{} {}/{} → {} ({}) {}",
                    ts, proc_prefix, n, svc, meth, status, latency, addr
                ),
                _ => println!(
                    "[{}] {}\x1b[34mHTTP/2\x1b[0m #{} {} {} → {} ({}) {}",
                    ts, proc_prefix, n, method, path, status, latency, addr
                ),
            }
        }

        None => {
            // Raw TCP/UDP flow — show in verbose mode or when process-attributed (eBPF)
            if std::env::var("NETSCOPE_VERBOSE").is_ok() || flow.process.is_some() {
                println!(
                    "[{}] {}{} #{} {}:{} → {}:{}",
                    ts, proc_prefix, flow.protocol, n,
                    flow.src_ip, flow.src_port, flow.dst_ip, flow.dst_port
                );
            }
        }
    }
}
