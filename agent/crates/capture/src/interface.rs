#[derive(Debug, Clone)]
pub struct InterfaceInfo {
    pub name: String,
    pub description: String,
    pub addresses: Vec<String>,
}

impl std::fmt::Display for InterfaceInfo {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.name)?;
        if !self.addresses.is_empty() {
            write!(f, " ({})", self.addresses.join(", "))?;
        }
        if !self.description.is_empty() {
            write!(f, " — {}", self.description)?;
        }
        Ok(())
    }
}
