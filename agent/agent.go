package agent

import (
    "context"
    "log"
    "net/http"
    "os"
    "time"

    "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
    "go.opentelemetry.io/contrib/instrumentation/runtime"
    "go.opentelemetry.io/contrib/propagators/b3"
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
    "go.opentelemetry.io/otel/sdk/resource"
    sdkmetric "go.opentelemetry.io/otel/sdk/metric"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    "go.opentelemetry.io/otel/semconv/v1.21.0"
    "github.com/gorilla/mux"
)

func StartAgent() *mux.Router {
    ctx := context.Background()

    serviceName := os.Getenv("SERVICE_NAME")
    if serviceName == "" {
        serviceName = "default-service"
    }
    serviceVersion := os.Getenv("SERVICE_VERSION")
    if serviceVersion == "" {
        serviceVersion = "1.0.0"
    }
    deploymentEnvironment := os.Getenv("DEPLOYMENT_ENVIRONMENT")
    if deploymentEnvironment == "" {
        deploymentEnvironment = "development"
    }

    resources, err := resource.New(ctx,
        resource.WithAttributes(
            semconv.ServiceNameKey.String(serviceName),
            semconv.ServiceVersionKey.String(serviceVersion),
            semconv.DeploymentEnvironmentKey.String(deploymentEnvironment),
        ),
    )
    if err != nil {
        log.Fatalf("failed to create resource: %v", err)
    }

    traceExporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")))
    if err != nil {
        log.Fatalf("failed to create trace exporter: %v", err)
    }

    metricExporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpoint(os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT")))
    if err != nil {
        log.Fatalf("failed to create metric exporter: %v", err)
    }

    bsp := sdktrace.NewBatchSpanProcessor(traceExporter)
    tracerProvider := sdktrace.NewTracerProvider(
        sdktrace.WithResource(resources),
        sdktrace.WithSpanProcessor(bsp),
    )
    otel.SetTracerProvider(tracerProvider)

    meterProvider := sdkmetric.NewMeterProvider(
        sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
        sdkmetric.WithResource(resources),
    )
    otel.SetMeterProvider(meterProvider)

    propagators := b3.New()
    otel.SetTextMapPropagator(propagators)

    if err := runtime.Start(runtime.WithMinimumReadMemStatsInterval(time.Second)); err != nil {
        log.Fatalf("failed to start runtime instrumentation: %v", err)
    }

    router := mux.NewRouter()
    router.Use(func(handler http.Handler) http.Handler {
        return otelhttp.NewHandler(handler, "http-server")
    })

    return router
}
