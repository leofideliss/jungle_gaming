package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

type contextKey string

const ProviderID contextKey = "providerId"

type claims struct {
	Azp string `json:"azp"`
}

func NewAuthMiddleware(ctx context.Context, issuerUrl string) (func(http.Handler) http.Handler, error) {
	provider, err := oidc.NewProvider(ctx, issuerUrl)
	if err != nil {
		return nil, err
	}

	verifier := provider.Verifier(&oidc.Config{
		SkipClientIDCheck: true,
	})

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "token ausente", http.StatusUnauthorized)
				return
			}
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, "formato de token invalido", http.StatusUnauthorized)
				return

			}
			idToken, err := verifier.Verify(r.Context(), parts[1])
			if err != nil {
				http.Error(w, "token invalido", http.StatusUnauthorized)
				return
			}

			var c claims
			if err := idToken.Claims(&c); err != nil {
				http.Error(w, "claims invalido", http.StatusUnauthorized)
				return
			}

			if c.Azp == "" {
				http.Error(w, "providerId ausente", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), ProviderID, c.Azp)
			next.ServeHTTP(w, r.WithContext(ctx))

		})
	}, nil
}
