package exporter

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	adexchangebuyer "google.golang.org/api/adexchangebuyer2/v2beta1"
	googleOption "google.golang.org/api/option"
)

const Namespace = "google_adexchangebuyerapi" // For Prometheus metrics.

type metricInfo struct {
	Desc *prometheus.Desc
	Type prometheus.ValueType
}

func newMetric(metricName, metricDomain, docString string, t prometheus.ValueType, constLabels prometheus.Labels, variableLabels ...string) metricInfo {
	return metricInfo{
		Desc: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, metricDomain, metricName),
			docString,
			variableLabels,
			constLabels,
		),
		Type: t,
	}
}

type metrics map[int]metricInfo

var (
	bidResponseErrorMetrics        = metrics{}
	bidResponsesWithoutBidsMetrics = metrics{}
	filteredBidRequestsMetrics     = metrics{}
	filteredBidsMetrics            = metrics{}
	losingBidsMetrics              = metrics{}
	nonBillableWinningBidsMetrics  = metrics{}

	bidsMetrics = metrics{
		1: newMetric("bids_total", "", "Total number of bids received from the buyer", prometheus.CounterValue, nil),
		2: newMetric("bids_in_auction_total", "", "Total number of bids that are in the auction", prometheus.CounterValue, nil),
		3: newMetric("impressions_won_total", "", "Total number of bids won the auction", prometheus.CounterValue, nil),
		4: newMetric("reached_queries_total", "", "Total number of bids won the auction and the mediation waterfall (if any)", prometheus.CounterValue, nil),
		5: newMetric("billed_impressions_total", "", "Total number of bids for which the buyer was billed", prometheus.CounterValue, nil),
		6: newMetric("measurable_impressions_total", "", "Total number of bids for which the corresponding impression was measurable for viewability", prometheus.CounterValue, nil),
		7: newMetric("viewable_impressions_total", "", "Total number of bids for which the corresponding impression was viewable", prometheus.CounterValue, nil),
	}

	impressionsMetrics = metrics{
		1: newMetric("available_impressions_total", "", "Total number of impressions available to the buyer", prometheus.CounterValue, nil),
		2: newMetric("inventory_matches_total", "", "Total number of impressions that match the buyer's inventory pretargeting", prometheus.CounterValue, nil),
		3: newMetric("bid_requests_total", "", "Total number of impressions for which the Ad Exchange sent the buyer a bid request", prometheus.CounterValue, nil),
		4: newMetric("successful_responses_total", "", "Total number of impressions for which the buyer sucessfully sent a response to the Ad Exchange", prometheus.CounterValue, nil),
		5: newMetric("responses_with_bids_total", "", "Total number of impressions for which the Ad Exchange received an applicable bid from the buyer", prometheus.CounterValue, nil),
	}

	adexchangebuyerInfo = prometheus.NewDesc(prometheus.BuildFQName(Namespace, "version", "info"), "Ad Exchange Buyer API version info.", []string{"version"}, nil)
	adexchangebuyerUp   = prometheus.NewDesc(prometheus.BuildFQName(Namespace, "", "up"), "Was the last scrape of the Ad Exchange Buyer API successful.", nil, nil)
)

// Exporter collects stats from the Ad Exchange Buyer API
// with the given credentials and exports them using
// the prometheus metrics package.
type Exporter struct {
	service   *adexchangebuyer.Service
	bidderID  string
	filterSet string
	timeout   time.Duration
	mutex     sync.RWMutex

	up                                  prometheus.Gauge
	totalScrapes, responseParseFailures prometheus.Counter
	logger                              log.Logger
}

// NewExporter returns an initialized Exporter.
func NewExporter(serviceAccount, bidderID string, timeout time.Duration, logger log.Logger) (*Exporter, error) {
	ctx := context.Background()
	svc, err := adexchangebuyer.NewService(ctx, googleOption.WithCredentialsJSON([]byte(serviceAccount)), googleOption.WithTelemetryDisabled())
	if err != nil {
		return nil, err
	}

	bidderFilterSet := &adexchangebuyer.FilterSet{
		Name:              fmt.Sprintf("bidders/%s/filterSets/_Exporter_FilterSet_%d", bidderID, time.Now().Unix()),
		RealtimeTimeRange: &adexchangebuyer.RealtimeTimeRange{StartTimestamp: time.Now().Format(time.RFC3339)},
	}
	createdBidderFilterSet, err := svc.Bidders.FilterSets.Create(fmt.Sprintf("bidders/%s", bidderID), bidderFilterSet).IsTransient(true).Do()
	if err != nil {
		return nil, err
	}

	initMetricsWithStatuses()

	return &Exporter{
		service:   svc,
		bidderID:  bidderID,
		filterSet: createdBidderFilterSet.Name,
		timeout:   timeout,
		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "up",
			Help:      "Was the last scrape of the Ad Exchange Buyer API successful.",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "exporter_scrapes_total",
			Help:      "Current total Ad Exchange Buyer API scrapes.",
		}),
		responseParseFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "exporter_response_parse_failures_total",
			Help:      "Number of errors while parsing responses.",
		}),
		logger: logger,
	}, nil
}

func initMetricsWithStatuses() {
	for status, reason := range calloutStatuses {
		bidResponseErrorMetrics[status] = newMetric("bid_response_errors_total", "", "Total number of bid response errors", prometheus.CounterValue, prometheus.Labels{"reason": reason})
		filteredBidRequestsMetrics[status] = newMetric("filtered_bid_requests_total", "", "Total number of filtered bid requests", prometheus.CounterValue, prometheus.Labels{"reason": reason})
	}

	for _, v := range bidResponsesWithoutBidsStatuses {
		bidResponsesWithoutBidsMetrics[v.idx] = newMetric("bid_response_without_bids_total", "", "Total number of bid responses without bids", prometheus.CounterValue, prometheus.Labels{"reason": v.reason})
	}

	for status, reason := range creativeStatuses {
		filteredBidsMetrics[status] = newMetric("filtered_bids_total", "", "Total number of filtered bids", prometheus.CounterValue, prometheus.Labels{"reason": reason})
		losingBidsMetrics[status] = newMetric("losing_bids_total", "", "Total number of losing bids", prometheus.CounterValue, prometheus.Labels{"reason": reason})
	}

	for _, v := range nonBillableWinningBidsStatuses {
		nonBillableWinningBidsMetrics[v.idx] = newMetric("non_billable_winning_bids_total", "", "Total number of non billable winning bids", prometheus.CounterValue, prometheus.Labels{"reason": v.reason})
	}
}

func (e *Exporter) refreshFilterSet() {
	go func() {
		t := time.NewTicker(time.Minute * 45)
		for range t.C {
			level.Info(e.logger).Log("msg", "Refreshing the filterset name")

			bidderFilterSet := &adexchangebuyer.FilterSet{
				Name:              fmt.Sprintf("bidders/%s/filterSets/_Exporter_FilterSet_%d", e.bidderID, time.Now().Unix()),
				RealtimeTimeRange: &adexchangebuyer.RealtimeTimeRange{StartTimestamp: time.Now().Format(time.RFC3339)},
			}
			createdBidderFilterSet, err := e.service.Bidders.FilterSets.Create(fmt.Sprintf("bidders/%s", e.bidderID), bidderFilterSet).IsTransient(true).Do()
			if err != nil {
				level.Error(e.logger).Log("msg", "Error creating filterSet", "err", err)
				return
			}
			e.filterSet = createdBidderFilterSet.Name
		}
	}()
}

// Describe describes all the metrics ever exported by the Ad Exchange Buyer API exporter.
// It implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range bidResponseErrorMetrics {
		ch <- m.Desc
	}

	for _, m := range bidsMetrics {
		ch <- m.Desc
	}

	for _, m := range bidResponsesWithoutBidsMetrics {
		ch <- m.Desc
	}

	for _, m := range filteredBidRequestsMetrics {
		ch <- m.Desc
	}

	for _, m := range filteredBidsMetrics {
		ch <- m.Desc
	}

	for _, m := range impressionsMetrics {
		ch <- m.Desc
	}

	for _, m := range losingBidsMetrics {
		ch <- m.Desc
	}

	for _, m := range nonBillableWinningBidsMetrics {
		ch <- m.Desc
	}

	ch <- adexchangebuyerInfo
	ch <- adexchangebuyerUp
	ch <- e.totalScrapes.Desc()
	ch <- e.responseParseFailures.Desc()
}

// Collect fetches the stats from configured Ad Exchange Buyer API account
//  and delivers them as Prometheus metrics. It implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.Lock() // To protect metrics from concurrent collects.
	defer e.mutex.Unlock()

	up := e.scrape(ch)

	ch <- prometheus.MustNewConstMetric(adexchangebuyerUp, prometheus.GaugeValue, up)
	ch <- e.totalScrapes
	ch <- e.responseParseFailures
}

func (e *Exporter) scrape(ch chan<- prometheus.Metric) (up float64) {
	e.totalScrapes.Inc()
	ch <- prometheus.MustNewConstMetric(adexchangebuyerInfo, prometheus.GaugeValue, 1, e.service.BasePath)

	wg := &sync.WaitGroup{}
	wg.Add(8)

	go func() {
		defer wg.Done()
		e.exportBids(ch)
	}()

	go func() {
		defer wg.Done()
		e.exportBidResponseErrors(ch)
	}()

	go func() {
		defer wg.Done()
		e.exportBidResponsesWithoutBids(ch)
	}()

	go func() {
		defer wg.Done()
		e.exportFilteredBidRequests(ch)
	}()

	go func() {
		defer wg.Done()
		e.exportFilteredBids(ch)
	}()

	go func() {
		defer wg.Done()
		e.exportImpressionMetrics(ch)
	}()

	go func() {
		defer wg.Done()
		e.exportLosingBids(ch)
	}()

	go func() {
		defer wg.Done()
		e.exportNonBillableWinningBids(ch)
	}()

	wg.Wait()

	return 1
}

func (e *Exporter) exportBids(ch chan<- prometheus.Metric) error {
	response, err := e.service.Bidders.FilterSets.BidMetrics.List(e.filterSet).Do()
	if err != nil {
		level.Error(e.logger).Log("msg", "Can't scrape Ad Exchange Buyer API for Bid Metrics", "err", err)
		return err
	}

	for _, v := range response.BidMetricsRows {
		e.exportMetric(ch, bidsMetrics[1], v.Bids)
		e.exportMetric(ch, bidsMetrics[2], v.BidsInAuction)
		e.exportMetric(ch, bidsMetrics[3], v.ImpressionsWon)
		e.exportMetric(ch, bidsMetrics[4], v.ReachedQueries)
		e.exportMetric(ch, bidsMetrics[5], v.BilledImpressions)
		e.exportMetric(ch, bidsMetrics[6], v.MeasurableImpressions)
		e.exportMetric(ch, bidsMetrics[7], v.ViewableImpressions)
	}

	return nil
}

func (e *Exporter) exportBidResponseErrors(ch chan<- prometheus.Metric) error {
	response, err := e.service.Bidders.FilterSets.BidResponseErrors.List(e.filterSet).Do()
	if err != nil {
		level.Error(e.logger).Log("msg", "Can't scrape Ad Exchange Buyer API Bid Response Errors", "err", err)
		return err
	}

	for _, v := range response.CalloutStatusRows {
		e.exportMetric(ch, bidResponseErrorMetrics[int(v.CalloutStatusId)], v.ImpressionCount)
	}

	return nil
}

func (e *Exporter) exportBidResponsesWithoutBids(ch chan<- prometheus.Metric) error {
	response, err := e.service.Bidders.FilterSets.BidResponsesWithoutBids.List(e.filterSet).Do()
	if err != nil {
		level.Error(e.logger).Log("msg", "Can't scrape Ad Exchange Buyer API for Bid Responses Without Bids", "err", err)
		return err
	}

	for _, v := range response.BidResponseWithoutBidsStatusRows {
		e.exportMetric(ch, bidResponsesWithoutBidsMetrics[bidResponsesWithoutBidsStatuses[v.Status].idx], v.ImpressionCount)
	}

	return nil
}

func (e *Exporter) exportFilteredBidRequests(ch chan<- prometheus.Metric) error {
	response, err := e.service.Bidders.FilterSets.FilteredBidRequests.List(e.filterSet).Do()
	if err != nil {
		level.Error(e.logger).Log("msg", "Can't scrape Ad Exchange Buyer API for Filtered Bid Requests", "err", err)
		return err
	}

	for _, v := range response.CalloutStatusRows {
		e.exportMetric(ch, filteredBidRequestsMetrics[int(v.CalloutStatusId)], v.ImpressionCount)
	}

	return nil
}

func (e *Exporter) exportFilteredBids(ch chan<- prometheus.Metric) error {
	response, err := e.service.Bidders.FilterSets.FilteredBids.List(e.filterSet).Do()
	if err != nil {
		level.Error(e.logger).Log("msg", "Can't scrape Ad Exchange Buyer API for Filtered Bids", "err", err)
		return err
	}

	for _, v := range response.CreativeStatusRows {
		e.exportMetric(ch, filteredBidsMetrics[int(v.CreativeStatusId)], v.BidCount)
	}

	return nil
}

func (e *Exporter) exportImpressionMetrics(ch chan<- prometheus.Metric) error {
	response, err := e.service.Bidders.FilterSets.ImpressionMetrics.List(e.filterSet).Do()
	if err != nil {
		level.Error(e.logger).Log("msg", "Can't scrape Ad Exchange Buyer API for Impression Metrics", "err", err)
		return err
	}

	for _, v := range response.ImpressionMetricsRows {
		e.exportMetric(ch, impressionsMetrics[1], v.AvailableImpressions)
		e.exportMetric(ch, impressionsMetrics[2], v.InventoryMatches)
		e.exportMetric(ch, impressionsMetrics[3], v.BidRequests)
		e.exportMetric(ch, impressionsMetrics[4], v.SuccessfulResponses)
		e.exportMetric(ch, impressionsMetrics[5], v.ResponsesWithBids)
	}

	return nil
}

func (e *Exporter) exportLosingBids(ch chan<- prometheus.Metric) error {
	response, err := e.service.Bidders.FilterSets.LosingBids.List(e.filterSet).Do()
	if err != nil {
		level.Error(e.logger).Log("msg", "Can't scrape Ad Exchange Buyer API for Losing Bids", "err", err)
		return err
	}

	for _, v := range response.CreativeStatusRows {
		e.exportMetric(ch, losingBidsMetrics[int(v.CreativeStatusId)], v.BidCount)
	}

	return nil
}

func (e *Exporter) exportNonBillableWinningBids(ch chan<- prometheus.Metric) error {
	response, err := e.service.Bidders.FilterSets.NonBillableWinningBids.List(e.filterSet).Do()
	if err != nil {
		level.Error(e.logger).Log("msg", "Can't scrape Ad Exchange Buyer API for Non-Billable Winning Bids", "err", err)
		return err
	}

	for _, v := range response.NonBillableWinningBidStatusRows {
		e.exportMetric(ch, nonBillableWinningBidsMetrics[nonBillableWinningBidsStatuses[v.Status].idx], v.BidCount)
	}

	return nil
}

func (e *Exporter) exportMetric(ch chan<- prometheus.Metric, metric metricInfo, value *adexchangebuyer.MetricValue) {
	// The Desc doesn't exist so the metric doesn't exist
	if metric.Desc == nil {
		e.responseParseFailures.Inc()
		return
	}

	ch <- prometheus.MustNewConstMetric(metric.Desc, metric.Type, float64(value.Value))
}
