package agent

import (
	"context"
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
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

func StartAgent() *mux.Router {
	ctx := context.Background()

	// Configura o serviço e o ambiente
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

	// Configura o recurso do serviço com atributos globais
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

	// Configura o exportador de traces com a URL completa
	traceExporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")))
	if err != nil {
		log.Fatalf("failed to create trace exporter: %v", err)
	}

	// Configura o exportador de métricas com a URL completa
	metricExporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT")))
	if err != nil {
		log.Fatalf("failed to create metric exporter: %v", err)
	}

	// Configura o provedor de trace
	bsp := sdktrace.NewBatchSpanProcessor(traceExporter)
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(resources),
		sdktrace.WithSpanProcessor(bsp),
	)
	otel.SetTracerProvider(tracerProvider)

	// Configura o provedor de métricas
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(resources),
	)
	otel.SetMeterProvider(meterProvider)

	// Configura propagadores (B3 como padrão)
	propagators := b3.New()
	otel.SetTextMapPropagator(propagators)

	// Instrumentação de runtime
	if err := runtime.Start(runtime.WithMinimumReadMemStatsInterval(time.Second)); err != nil {
		log.Fatalf("failed to start runtime instrumentation: %v", err)
	}

	// Configuração do mux com auto-instrumentação
	router := mux.NewRouter()

	// Middleware para adicionar atributos personalizados
	router.Use(otelhttp.NewMiddleware("http-server", otelhttp.WithTracerProvider(tracerProvider)))
	router.Use(customMiddleware)

	return router
}

func customMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Inicia um span manualmente, se necessário, ou usa o span já existente
		span := trace.SpanFromContext(r.Context())
		span.SetAttributes(
			attribute.String("http.method", r.Method),
			attribute.String("http.path", r.URL.Path),
			attribute.String("http.url", r.URL.String()),
			attribute.String("http.user_agent", r.UserAgent()),
			attribute.String("http.client_ip", r.RemoteAddr),
		)

		next.ServeHTTP(w, r)
	})
}
