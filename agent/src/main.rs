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

fn print_flow(flow: &proto::Flow, n: u64) {
    let ts = flow.timestamp.format("%H:%M:%S%.3f");

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
                "[{}] \x1b[34mHTTP\x1b[0m #{} {} {} {} → {} ({}) {}",
                ts, n, method, host, path, status, latency,
                format!("{}:{} → {}:{}", flow.src_ip, flow.src_port, flow.dst_ip, flow.dst_port)
            );

            // Print request headers at debug verbosity
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
                    "[{}] \x1b[35mDNS\x1b[0m #{} {} {} {} → [{}]",
                    ts,
                    n,
                    direction,
                    dns.query_name,
                    dns.query_type,
                    answers.join(", ")
                );
            } else if !dns.is_response {
                println!(
                    "[{}] \x1b[35mDNS\x1b[0m #{} {} {} {}",
                    ts, n, direction, dns.query_name, dns.query_type
                );
            }
        }

        None => {
            // Raw TCP/UDP flow with no decoded payload — only shown in verbose mode
            if std::env::var("NETSCOPE_VERBOSE").is_ok() {
                println!(
                    "[{}] {} #{} {}:{} → {}:{}",
                    ts,
                    flow.protocol,
                    n,
                    flow.src_ip,
                    flow.src_port,
                    flow.dst_ip,
                    flow.dst_port
                );
            }
        }
    }
}
