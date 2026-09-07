package cli

import "runtime/debug"

// Version is set by release builds using -ldflags -X.
var Version string

func commandVersion() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
