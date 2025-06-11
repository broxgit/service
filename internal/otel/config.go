package otel

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/broxgit/log"
)

// SetGlobalTracerProvider configures an OpenTelemetry exporter and trace provider.
func SetGlobalTracerProvider(ctx context.Context) {
	address := fmt.Sprintf("%v:%v", getOtelHost(), getOtelPort())
	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(address), otlptracegrpc.WithInsecure())
	if err != nil {
		log.ZError("failed to create an otel exporter", log.Z("err", err))
		return
	}
	setServiceName()
	tracerprovider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
	)
	otel.SetTracerProvider(tracerprovider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
}

func setServiceName() {
	if os.Getenv("OTEL_SERVICE_NAME") != "" {
		return
	}
	os.Setenv("OTEL_SERVICE_NAME", getServiceName())
}

func getServiceName() string {
	podMatcher := regexp.MustCompile("^(.+)-[0-9a-f]{1,10}-[0-9a-z]{5}$")
	hostname := os.Getenv("HOSTNAME")
	deploymentName := podMatcher.FindStringSubmatch(hostname)
	if len(deploymentName) > 0 {
		return deploymentName[len(deploymentName)-1] // regex matches the entire string. We want the most specific (last) match.
	} else if len(hostname) > 0 {
		return hostname
	}
	return "unknown"
}

func getOtelHost() string {
	host := os.Getenv("OTEL_COLLECTOR_HOST")
	if host == "" {
		log.ZDebug("OTEL enabled without setting OTEL_COLLECTOR_HOST. Using default.")
		return "collector-collector.otel"
	}
	return host
}

func getOtelPort() int {
	const defaultPort = 4317
	port := os.Getenv("OTEL_COLLECTOR_PORT")
	if port == "" {
		log.ZDebug("OTEL enabled without setting OTEL_COLLECTOR_PORT. Using default.")
		return defaultPort
	}
	portNum, err := strconv.Atoi(port)
	if err != nil {
		log.ZError("An invalid port set for OTEL_COLLECTOR_HOST", log.Z("port", port))
		return defaultPort
	}
	return portNum
}
