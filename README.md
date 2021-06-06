# Google Ad Exchange Buyer Exporter

This is an exporter for the Google Ad Exchange Buyer API II. It exports all of the available metrics from the API, in order to allow you to build a view similar to the RTB Breakout Troubleshooting view in the Realtime Bidding Console.

## Getting Started

To run it:

```bash
./google-adexchangebuyer-exporter --google.service-account="$(GOOGLE_SERVICE_ACCOUNT)" --google.bidder-id="$(GOOGLE_BIDDER_ID)"
```

Where `GOOGLE_SERVICE_ACCOUNT` is a service account that can access the Google Ad Exchange Buyer API II, and `GOOGLE_BIDDER_ID` is the bidder ID that you are trying to extract the data for.

## Docker

To run the exporter as a Docker container, run:

```bash
docker run -p 9835:9835 probablynot/google-adexchangebuyer-exporter --google.service-account="$(GOOGLE_SERVICE_ACCOUNT)" --google.bidder-id="$(GOOGLE_BIDDER_ID)"
```
