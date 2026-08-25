package execution

import "strings"

type Mode string

const (
	ModeOff       Mode = "off"
	ModeWorkspace Mode = "workspace"
	ModeContainer Mode = "container"
)

type NetworkMode string

const (
	NetworkOff       NetworkMode = "off"
	NetworkOn        NetworkMode = "on"
	NetworkAllowlist NetworkMode = "allowlist"
)

type Options struct {
	Mode              Mode
	Image             string
	Network           NetworkMode
	AllowEnvKeys      []string
	ContainerRuntime  Runtime
	FallbackWorkspace bool
	AllowDomains      []string
	TimeoutSec        int
}

func ParseMode(s string) Mode {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "workspace":
		return ModeWorkspace
	case "container":
		return ModeContainer
	default:
		return ModeOff
	}
}

func ParseNetworkMode(s string) NetworkMode {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "on":
		return NetworkOn
	case "allowlist":
		return NetworkAllowlist
	default:
		return NetworkOff
	}
}
