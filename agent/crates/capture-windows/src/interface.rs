//! Windows network interface enumeration via Npcap.

#![cfg(target_os = "windows")]

use anyhow::Result;
use pcap::Device;

/// A simplified description of a network interface.
#[derive(Debug, Clone)]
pub struct Interface {
    pub name:        String,
    pub description: String,
}

/// List all interfaces visible to Npcap.
pub fn list() -> Result<Vec<Interface>> {
    let devices = Device::list()?;
    Ok(devices
        .into_iter()
        .map(|d| Interface {
            description: d.desc.clone().unwrap_or_default(),
            name: d.name,
        })
        .collect())
}

/// Find an interface by name or return the first available one.
pub fn find(name: Option<&str>) -> Result<Device> {
    let devices = Device::list()?;
    if let Some(target) = name {
        for d in &devices {
            if d.name == target {
                return Ok(d.clone());
            }
        }
        anyhow::bail!("Interface '{}' not found. Available: {:?}",
            target,
            devices.iter().map(|d| &d.name).collect::<Vec<_>>()
        );
    }
    // Default to first available.
    devices.into_iter().next()
        .ok_or_else(|| anyhow::anyhow!("No network interfaces found via Npcap"))
}
