package agent

import (
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func tracingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Inicia um novo span para a requisição
		ctx, span := otel.Tracer("http-server").Start(r.Context(), r.URL.Path)
		defer span.End()

		// Adiciona atributos ao span
		span.SetAttributes(
			attribute.String("http.method", r.Method),
			attribute.String("http.path", r.URL.Path),
		)

		// Passa o contexto atualizado para a próxima fase do middleware
		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	})
}
