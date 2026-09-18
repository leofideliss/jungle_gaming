# Jungle Gaming

Serviço em Go para processamento de operações financeiras de apostas. Esta entrega cobre
o núcleo financeiro (Money, Wallet, ledger) e o controle de concorrência sobre o saldo,
com PostgreSQL e migrations versionadas. O restante do escopo está descrito no
ARCHITECTURE.

## Requisitos

- Docker e Docker Compose
- Go 1.22+
- `build-essential` (gcc), necessário só para rodar os testes com `-race`

## Rodando

```sh
cp .env.example .env
docker compose up postgres migrate
```

O serviço `migrate` espera o banco ficar pronto, aplica as migrations e encerra.

## Migrations

Versionadas com golang-migrate em `migrations/` (pares up/down). Reverter tudo:

```sh
docker compose run --rm migrate -path /migrations \
  -database "$DATABASE_URL" down -all
```

Recriar do zero:

```sh
docker compose down -v && docker compose up postgres migrate
```

## Testes

```sh
go test ./...                          # tudo (integração exige o banco no ar)
go test ./internal/domain/...          # só domínio, sem banco
CGO_ENABLED=1 go test -race ./...      # com detector de corrida
```

O cenário de concorrência (duas apostas de 80 sobre saldo de 100) está em
`internal/repository`:

```sh
CGO_ENABLED=1 go test -race -run TestConcurrentDebits ./internal/repository/
```

## Estrutura

```
internal/domain/money    valor monetário em int64 (centavos)
internal/domain/wallet   carteira, débito/crédito, invariantes
internal/domain/ledger   lançamento imutável
internal/repository      persistência com pgx
migrations               schema versionado
```
