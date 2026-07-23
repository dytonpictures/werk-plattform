package httpapi

import (
	"context"
	"net/http"

	"github.com/dytonpictures/werk/internal/core/identity"
)

type actorContextKey struct{}

// WithAuthenticatedActor is used only by the trusted authentication adapter
// after it has resolved a server-side session.
func WithAuthenticatedActor(ctx context.Context, actor identity.AuthenticatedActor) context.Context {
	return context.WithValue(ctx, actorContextKey{}, actor)
}

func authenticatedActorFromContext(ctx context.Context) (identity.AuthenticatedActor, bool) {
	actor, ok := ctx.Value(actorContextKey{}).(identity.AuthenticatedActor)
	return actor, ok
}

// RequireAccessPlane enforces an API boundary independently of frontend routes
// or menu visibility. Missing or mismatched actors fail closed.
func RequireAccessPlane(expected identity.AccessPlane) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			actor, ok := authenticatedActorFromContext(request.Context())
			if !ok || identity.AuthorizeAccessPlane(actor, expected) != nil {
				writeProblem(writer, request, http.StatusForbidden, "access-denied", "Access denied", "The session is not authorized for this API area.")
				return
			}
			next.ServeHTTP(writer, request)
		})
	}
}
