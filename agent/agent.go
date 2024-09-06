package agent

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// StartAgent configura e inicia o agente OpenTelemetry para a aplicação.
func StartAgent(config Config) *mux.Router {
	ctx := context.Background()

	serviceName := config.ServiceName
	if serviceName == "" {
		serviceName = "default-service"
	}
	serviceVersion := config.ServiceVersion
	if serviceVersion == "" {
		serviceVersion = "1.0.0"
	}
	deploymentEnvironment := config.DeploymentEnvironment
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

	traceExporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(config.TraceEndpoint))
	if err != nil {
		log.Fatalf("failed to create trace exporter: %v", err)
	}

	metricExporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(config.MetricEndpoint))
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

	propagators := propagation.NewCompositeTextMapPropagator(
		b3.New(),
		propagation.TraceContext{},
		propagation.Baggage{},
	)
	otel.SetTextMapPropagator(propagators)

	if err := runtime.Start(runtime.WithMinimumReadMemStatsInterval(time.Second)); err != nil {
		log.Fatalf("failed to start runtime instrumentation: %v", err)
	}

	router := mux.NewRouter()
	router.Use(otelhttp.NewMiddleware(
		"http-server",
		otelhttp.WithTracerProvider(tracerProvider),
		otelhttp.WithPropagators(propagators),
		otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
			return r.Method + " " + r.URL.Path
		}),
	))

	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, span := otel.Tracer("http-server").Start(r.Context(), r.Method+" "+r.URL.Path)
			defer span.End()

			span.SetAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.path", r.URL.Path),
				attribute.String("http.url", r.URL.String()),
				attribute.String("http.user_agent", r.UserAgent()),
				attribute.String("http.client_ip", r.RemoteAddr),
			)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})

	return router
}

// GetHTTPClient retorna um cliente HTTP com transporte instrumentado para propagação de trace.
func GetHTTPClient() *http.Client {
	tr := otelhttp.NewTransport(http.DefaultTransport, otelhttp.WithPropagators(propagation.NewCompositeTextMapPropagator(
		b3.New(),
		propagation.TraceContext{},
		propagation.Baggage{},
	)))

	return &http.Client{
		Transport: tr,
	}
}

// GetRequestWithContext encapsula a criação de uma nova requisição HTTP com o contexto propagado.
func GetRequestWithContext(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	return req, nil
}

// ExecuteRequest encapsula a execução de uma requisição HTTP, propagando o context
func ExecuteRequest(ctx context.Context, client *http.Client, method, url string, body io.Reader) (*http.Response, error) {
	req, err := GetRequestWithContext(ctx, method, url, body)

	if err != nil {
		return nil, err
	}

	return client.Do(req)
}

// Config struct to hold the configuration parameters
type Config struct {
	ServiceName           string
	ServiceVersion        string
	DeploymentEnvironment string
	TraceEndpoint         string
	MetricEndpoint        string
}

// DefaultConfig provides a default configuration
func DefaultConfig() Config {
	return Config{
		ServiceName:           os.Getenv("SERVICE_NAME"),
		ServiceVersion:        os.Getenv("SERVICE_VERSION"),
		DeploymentEnvironment: os.Getenv("DEPLOYMENT_ENVIRONMENT"),
		TraceEndpoint:         os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"),
		MetricEndpoint:        os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT"),
	}
}
