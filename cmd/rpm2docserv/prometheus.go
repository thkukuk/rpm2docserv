package main

import (
	"html/template"
	"io"
	"time"
)

const metricsTmplContent = `
# HELP rpm2docserv_packages_total The total number of binary packages processed.
# TYPE rpm2docserv_packages_total gauge
rpm2docserv_packages_total {{ .Packages }}

# HELP rpm2docserv_packages_extracted Number of binary packages from which manpages were extracted.
# TYPE rpm2docserv_packages_extracted gauge
rpm2docserv_packages_extracted {{ .Stats.PackagesExtracted }}

# HELP rpm2docserv_manpages_rendered Number of manpages rendered to HTML
# TYPE rpm2docserv_manpages_rendered gauge
rpm2docserv_manpages_rendered {{ .Stats.ManpagesRendered }}

# HELP rpm2docserv_manpage_bytes Total number of bytes used by manpages (by format).
# TYPE rpm2docserv_manpage_bytes gauge
rpm2docserv_manpage_bytes{format="man"} {{ .Stats.ManpageBytes }}
rpm2docserv_manpage_bytes{format="html"} {{ .Stats.HTMLBytes }}

# HELP rpm2docserv_index_bytes Total number of bytes used for the auxserver index.
# TYPE rpm2docserv_index_bytes gauge
rpm2docserv_index_bytes {{ .Stats.IndexBytes }}

# HELP rpm2docserv_runtime Wall-clock runtime in seconds.
# TYPE rpm2docserv_runtime gauge
rpm2docserv_runtime {{ .Seconds }}

# HELP rpm2docserv_last_successful_run Last successful run in seconds since the epoch.
# TYPE rpm2docserv_last_successful_run gauge
rpm2docserv_last_successful_run {{ .LastSuccessfulRun }}
`

var metricsTmpl = template.Must(template.New("metrics").Parse(metricsTmplContent))

func writeMetrics(w io.Writer, gv globalView, start time.Time) error {
	now := time.Now()
	return metricsTmpl.Execute(w, struct {
		Packages          int
		Stats             *stats
		Now               time.Time
		Seconds           int
		LastSuccessfulRun int64
	}{
		Packages:          len(gv.pkgs),
		Stats:             gv.stats,
		Now:               now,
		Seconds:           int(now.Sub(start).Seconds()),
		LastSuccessfulRun: now.Unix(),
	})
}
