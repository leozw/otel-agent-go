package agent

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
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

	// Configura o recurso do serviço
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

	// Configura o exportador de traces
	traceExporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")))
	if err != nil {
		log.Fatalf("failed to create trace exporter: %v", err)
	}

	// Configura o exportador de métricas
	metricExporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpoint(os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT")))
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

	// Configura propagadores
	propagators := b3.New()
	otel.SetTextMapPropagator(propagators)

	// Instrumentação de runtime
	if err := runtime.Start(runtime.WithMinimumReadMemStatsInterval(time.Second)); err != nil {
		log.Fatalf("failed to start runtime instrumentation: %v", err)
	}

	// Configurando o mux com auto-instrumentação
	router := mux.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, span := otel.Tracer("http-server").Start(r.Context(), r.URL.Path)
			defer span.End()

			// Configurando atributos manualmente
			span.SetAttributes(
				semconv.HTTPMethodKey.String(r.Method),
				semconv.HTTPTargetKey.String(r.URL.Path),
				semconv.UserAgentOriginalKey.String(r.UserAgent()),
				semconv.NetSockPeerAddrKey.String(r.RemoteAddr),
			)

			// Envolvendo a chamada original
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})

	return router
}
