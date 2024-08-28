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

// Config contém as configurações para o agente.
type Config struct {
	ServiceName           string
	ServiceVersion        string
	DeploymentEnvironment string
	TraceExporterURL      string
	MetricsExporterURL    string
}

// DefaultConfig retorna uma configuração padrão baseada em variáveis de ambiente.
func DefaultConfig() Config {
	return Config{
		ServiceName:           getEnv("SERVICE_NAME", "default-service"),
		ServiceVersion:        getEnv("SERVICE_VERSION", "1.0.0"),
		DeploymentEnvironment: getEnv("DEPLOYMENT_ENVIRONMENT", "development"),
		TraceExporterURL:      os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"),
		MetricsExporterURL:    os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT"),
	}
}

// StartAgent inicializa o agente com a configuração fornecida.
func StartAgent(config Config) *mux.Router {
	ctx := context.Background()

	// Configura o recurso do serviço
	resources, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(config.ServiceName),
			semconv.ServiceVersionKey.String(config.ServiceVersion),
			semconv.DeploymentEnvironmentKey.String(config.DeploymentEnvironment),
		),
	)
	if err != nil {
		log.Fatalf("failed to create resource: %v", err)
	}

	// Configura os exportadores de trace e métricas
	traceExporter := initTraceExporter(ctx, config.TraceExporterURL)
	metricExporter := initMetricExporter(ctx, config.MetricsExporterURL)

	// Configura o provedor de trace e métricas
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(resources),
		sdktrace.WithSpanProcessor(sdktrace.NewBatchSpanProcessor(traceExporter)),
	)
	otel.SetTracerProvider(tracerProvider)

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(resources),
	)
	otel.SetMeterProvider(meterProvider)

	// Configura propagadores
	propagators := propagation.NewCompositeTextMapPropagator(
		b3.New(),
		propagation.TraceContext{},
		propagation.Baggage{},
	)
	otel.SetTextMapPropagator(propagators)

	// Instrumentação de runtime
	if err := runtime.Start(runtime.WithMinimumReadMemStatsInterval(time.Second)); err != nil {
		log.Fatalf("failed to start runtime instrumentation: %v", err)
	}

	// Criação do roteador e adição dos middlewares
	router := mux.NewRouter()
	router.Use(otelhttp.NewMiddleware(
		"http-server",
		otelhttp.WithTracerProvider(tracerProvider),
		otelhttp.WithPropagators(propagators),
		otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
			return r.Method + " " + r.URL.Path
		}),
	))
	router.Use(LoggingMiddleware)

	return router
}

// GetHTTPClient retorna um cliente HTTP instrumentado.
func GetHTTPClient() *http.Client {
	return &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
}

// NewRequestWithContext cria uma nova requisição HTTP com o contexto propagado.
func NewRequestWithContext(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	return req, nil
}

// LoggingMiddleware adiciona logs e atributos ao span.
func LoggingMiddleware(next http.Handler) http.Handler {
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
}

// Funções auxiliares para inicializar os exportadores

func initTraceExporter(ctx context.Context, url string) sdktrace.SpanExporter {
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(url))
	if err != nil {
		log.Fatalf("failed to create trace exporter: %v", err)
	}
	return exporter
}

func initMetricExporter(ctx context.Context, url string) sdkmetric.Exporter {
	exporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(url))
	if err != nil {
		log.Fatalf("failed to create metric exporter: %v", err)
	}
	return exporter
}

// getEnv retorna o valor de uma variável de ambiente ou um valor padrão se não estiver definido.
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
