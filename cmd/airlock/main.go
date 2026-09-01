package main

import "github.com/mirelahmed-commits/SentinelAirlock/internal/cli"

var version = "dev"
var commit = "none"
var buildDate = "unknown"

func main() {
	cli.Version = version
	cli.Commit = commit
	cli.BuildDate = buildDate
	cli.Execute()
}
