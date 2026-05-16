package docker

// Shell command identifiers used across packages for invoking Docker
// (optionally prefixed with sudo). Centralized here so phases, chain, and
// cosmovisor packages don't redeclare the same strings.
const (
	CmdSudo   = "sudo"
	CmdDocker = "docker"
)
