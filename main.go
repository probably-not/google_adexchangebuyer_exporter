package main

import (
	"net/http"
	"net/http/pprof"
	"os"

	"github.com/go-kit/kit/log/level"
	"github.com/probably-not/google_adexchangebuyer_exporter/exporter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	webflag "github.com/prometheus/exporter-toolkit/web/kingpinflag"
	"gopkg.in/alecthomas/kingpin.v2"
)

const ExporterName = exporter.Namespace + "_exporter"

func main() {
	var (
		webConfig      = webflag.AddFlags(kingpin.CommandLine)
		listenAddress  = kingpin.Flag("web.listen-address", "Address to listen on for web interface and telemetry.").Default(":9835").String()
		metricsPath    = kingpin.Flag("web.telemetry-path", "Path under which to expose metrics.").Default("/metrics").String()
		serviceAccount = kingpin.Flag("google.service-account", "The service account to use in order to get the api data.").Default("").String()
		bidderID       = kingpin.Flag("google.bidder-id", "The bidder ID to use in order to get the api data.").Default("").String()
		apiTimeout     = kingpin.Flag("api.timeout", "Timeout for trying to get data from the API.").Default("5s").Duration()
	)

	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)
	kingpin.Version(version.Print(ExporterName))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger := promlog.New(promlogConfig)

	level.Info(logger).Log("msg", "Starting exporter", "name", ExporterName, "version", version.Info())
	level.Info(logger).Log("msg", "Build context", "context", version.BuildContext())
	level.Info(logger).Log("msg", "Flags", "listenAddress", *listenAddress, "metricsPath", *metricsPath, "serviceAccount", *serviceAccount, "bidderID", *bidderID, "apiTimeout", *apiTimeout)

	exporter, err := exporter.NewExporter(*serviceAccount, *bidderID, *apiTimeout, logger)
	if err != nil {
		level.Error(logger).Log("msg", "Error creating exporter", "err", err)
		os.Exit(1)
	}

	prometheus.MustRegister(exporter)
	prometheus.MustRegister(version.NewCollector("google_adexchangebuyerapi_exporter"))

	level.Info(logger).Log("msg", "Listening on address", "address", *listenAddress)

	mux := http.NewServeMux()

	// Explicit PProf handlers since we are not on the default http server
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// Real Routes
	mux.HandleFunc("/", rootHandler(*metricsPath))
	mux.Handle(*metricsPath, promhttp.Handler())

	srv := &http.Server{Addr: *listenAddress, Handler: mux}
	if err := web.ListenAndServe(srv, *webConfig, logger); err != nil {
		level.Error(logger).Log("msg", "Error starting HTTP server", "err", err)
		os.Exit(1)
	}
}

func rootHandler(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
						 <head><title>Google Adexchange Buyer API II Exporter</title></head>
						 <body>
						 <h1>Google Adexchange Buyer API II Exporter</h1>
						 <p><a href='` + path + `'>Metrics</a></p>
						 </body>
						 </html>`))
	}
}
