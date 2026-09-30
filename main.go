package main

import (
	"context"
	"fmt"
	"jungle_gaming/internal/config"
	"jungle_gaming/internal/handler"
	"jungle_gaming/internal/idempotency"
	"jungle_gaming/internal/middleware"
	"jungle_gaming/internal/repository"
	"jungle_gaming/internal/usecase"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"go.uber.org/fx"
)

func main() {
	godotenv.Load()
	fx.New(
		fx.Provide(
			func(lc fx.Lifecycle) (*pgxpool.Pool, error) {
				pool, err := config.NewPool()
				if err != nil {
					return nil, err
				}

				lc.Append(fx.Hook{
					OnStop: func(ctx context.Context) error {
						pool.Close()
						return nil
					},
				})
				return pool, nil
			},
		),
		fx.Provide(
			func() (func(http.Handler) http.Handler, error) {
				return middleware.NewAuthMiddleware(
					context.Background(),
					os.Getenv("KEYCLOAK_URL"),
				)
			}),
		fx.Provide(
			repository.NewWalletRepository,
			repository.NewOutboxRepository,
			repository.NewWagerTransactionRepository,
			func(r *repository.WagerTransactionRepository) *idempotency.IdempotencyService {
				return idempotency.NewIdempotencyService(r)
			},
			usecase.NewWagerUseCase,
			func(uc *usecase.WagerUseCase) *handler.WagerHandler {
				return handler.NewWagerHandler(uc)
			},
		),
		fx.Invoke(startServer),
	).Run()
}

func startServer(lc fx.Lifecycle, h *handler.WagerHandler, authMW func(http.Handler) http.Handler) {
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	wrapped := authMW(mux)

	server := &http.Server{
		Addr:    ":8080",
		Handler: wrapped,
	}
	lc.Append(fx.Hook{

		OnStart: func(ctx context.Context) error {
			fmt.Println("servidor HTTP em :8080")
			go server.ListenAndServe()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			fmt.Println("desligando servidor...")
			return server.Shutdown(ctx)
		},
	})
}
