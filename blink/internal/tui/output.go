package tui

// LineMsg is sent when a new line of output arrives from a service runner.
// Child is non-empty for nested processes (e.g. a docker compose container).
type LineMsg struct {
	Service string
	Child   string
	Line    string
}

// StatusMsg announces a service status change. Child is non-empty for nested
// processes managed by a runtime (e.g. docker compose containers).
type StatusMsg struct {
	Service string
	Child   string
	Status  string
	Err     error
	// Ports are the local TCP ports the service was observed to listen on,
	// carried on a service-level event (Child == "") by runtimes that discover
	// ports at startup (e.g. docker compose published ports). Empty otherwise.
	Ports []int
}

// WatchStatsMsg carries file and directory counts, both aggregate and
// per-service. Polled from the supervisor on a slow cadence; the footer renders
// the totals on the all-tab and the per-service entry on a service tab.
type WatchStatsMsg struct {
	Files  int
	Dirs   int
	PerSvc map[string]WatchStat
}

// WatchStat mirrors supervisor.WatchStat without leaking the import.
type WatchStat struct {
	Files int
	Dirs  int
}
