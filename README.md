# Google Ad Exchange Buyer Exporter

This is an exporter for the Google Ad Exchange Buyer API II. It exports all of the available metrics from the API, in order to allow you to build a view similar to the RTB Breakout Troubleshooting view in the Realtime Bidding Console.

In order to use this properly, you must first start by requesting a higher quota for the Ad Exchange Buyer API II. The base quota only allows 1000 queries per day, which is not enough if you want to be scraping near-realtime metrics. See [this page](https://developers.google.com/authorized-buyers/apis/limits) for more details of how to request a quota upgrade.

Each scrape of the metrics currently makes 8 requests to the API, and the default setting is to scrape once every 10 seconds. In order to have a high enough quota, I recommend requesting an increase to 100,000 requests per day for the __"RTB Troubleshooting per day"__ quota.

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

## Deploying in Kubernetes

This repo provides Kusomize manifests in order to deploy to Kubernetes. In order to deploy to Kubernetes, you can clone this repo and run:

```bash
kustomize build deploy/base | kubectl apply -f -
```

This will initialize all of the necessary resources. It is important to add a `config.env` file underneath the [base folder](/deploy/base) in order to generate the secrets file for the environment. The `config.env` file should look something like this:

```sh
GOOGLE_SERVICE_ACCOUNT=<<YOUR SERVICE ACCOUNT JSON>>
GOOGLE_BIDDER_ID=<<YOUR GOOGLE BIDDER ID>>
```

Without this `config.env` file, the deployment will not be able to get the necessary credentials to query the endpoints.

### Usage with the Prometheus Operator

There is a Kustomize overlay with a service monitor for usage with the Prometheus Operator. This will deploy a Service and a ServiceMonitor to allow the Prometheus Operator to discover the exporter. To use this deployment, you can run:

```bash
kustomize build deploy/prometheus-operator | kubectl apply -f -
```

### Usage with Standard Prometheus

If you are using this repo with the standard Prometheus setup as opposed to the Prometheus Operator, you may use the standard Prometheus overlay. To do this, run:

```bash
kustomize build deploy/standard-prometheus | kubectl apply -f -
```