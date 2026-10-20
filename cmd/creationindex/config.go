package main

type Config struct {
	Debug     bool     `help:"Enable debug logging (all categories)"`
	LogFilter []string `name:"log-filter" default:"startup,sync,status" help:"Log category filter (comma-separated)"`
	LogFile   string   `name:"log-file" help:"Log output file path (logs to both stdout and file when set)"`

	ActionStreamSource string `name:"actionstream-source" default:"stream://localhost:9081" help:"ActionStream source (stream://host:port or stream:///path/to.sock)"`
}
