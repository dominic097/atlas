package main

// Build metadata, injected at release time via:
//   -ldflags "-X main.Version=... -X main.Commit=... -X main.Date=..."
// These are surfaced through `atlas version`.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)
