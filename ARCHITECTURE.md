# Arquitetura

Núcleo de domínio (Money, Wallet, ledger) puro, sem dependência de infraestrutura, com
persistência em PostgreSQL. As invariantes financeiras são garantidas tanto no domínio
quanto nas constraints do banco.

## Money

Representado como `int64` em centavos. Optei por inteiro para evitar os problemas
conhecidos de ponto flutuante — o valor fica íntegro e a comunicação entre sistemas fica
mais previsível. Ponto flutuante não é usado em nenhuma etapa.

O parsing de decimal para centavos é feito por manipulação de string, com escala fixa de
2 casas, tratando overflow. Aceito formas equivalentes (`"25"`, `"25.5"`), normalizando
antes de qualquer hash de idempotência. A moeda segue ISO 4217, validada por um map de
moedas suportadas — o desafio usa só BRL, mas fica fácil adicionar outras. Os erros são
classificáveis por `errors.Is`.

## Domínio

Cada entidade separa criação de reidratação: criação valida a entrada externa e gera
id/versão; reidratação reconstrói o estado vindo do banco sem reaplicar movimentação.
Money e ledger são imutáveis; Wallet é entidade, e o saldo só muda pelos métodos
Credit/Debit, que validam as invariantes antes. O domínio não importa pgx, HTTP nem SQS.

## Persistência

Usei pgx com SQL explícito em vez de ORM, para manter transações, locks e constraints
visíveis e controláveis — tenho menos familiaridade com ORM em Go e prefiro não abrir mão
desse controle. Money é gravado como BIGINT (centavos) + CHAR(3). Migrations com
golang-migrate, aplicadas por um serviço do compose (não exige instalar nada na máquina),
na ordem wallets → transactions → ledger → inbox → outbox.

## Concorrência

Coordenação por carteira usando locking pessimista com `SELECT ... FOR UPDATE`. O débito
roda numa transação: lê a carteira com lock, valida saldo e monta o lançamento no domínio,
e grava saldo + ledger no mesmo commit.

Comecei com atualização atômica condicionada (`UPDATE ... WHERE balance >= x`), mas ela
não me dava o `balanceBefore` consistente para o ledger sob concorrência sem uma leitura
travada. O `FOR UPDATE` resolve: leitura e escrita sob o mesmo lock, então o
`balanceBefore` gravado corresponde ao saldo realmente usado. Como o lock é por linha,
carteiras diferentes seguem em paralelo, sem lock global.

Não-negatividade garantida em três camadas: validação no domínio, o `FOR UPDATE`, e
`CHECK (balance >= 0)` no schema.

O teste `TestConcurrentDebits_80_80_over_100` (`internal/repository`) dispara as duas
apostas de 80 em paralelo contra o Postgres real: uma processa, uma é rejeitada por saldo,
saldo final 20.00 e um único débito no ledger. Passa com `-race`.

## Invariantes no banco

- `CHECK (balance >= 0)`
- `UNIQUE (player_id, currency)` — uma carteira por jogador/moeda
- `UNIQUE (provider_id, external_transaction_id)` — idempotência de negócio
- índice único parcial de OPENING — impede crédito inicial duplicado
- `chk_origin_fields` — separa operação interna (OPENING) de externa
- `ck_after_value` — valida `after = before ± amount` conforme a direção
- `UNIQUE (wallet_id, transaction_id)` no ledger; FKs

O ledger é tratado como append-only (sem `updated_at`). A proteção definitiva contra
UPDATE/DELETE (trigger ou revogação de permissão) ficou como pendência.

## Idempotência

Identidade de negócio por `(providerId, externalTransactionId)`, com unicidade no schema,
pensada para ser persistente (nunca em memória). O schema já tem os campos
(`idempotency_key`, `payload_hash`, `result_balance`); a detecção de conflito seria por
hash canônico dos campos de negócio. O fluxo de aplicação não foi implementado.

## Não concluído

Por tempo, ficaram de fora — com o desenho pretendido:

- **HTTP e caso de uso** que orquestra carteira, ledger, idempotência e outbox num commit.
  O repositório e o débito concorrente já estão prontos para isso.
- **SQS + inbox**: entrada compartilhando o mesmo caso de uso, dedup por
  `(consumerName, messageId)` na mesma transação, remoção da fila só após commit.
- **Outbox**: eventos no mesmo commit, worker separado publicando com
  `FOR UPDATE SKIP LOCKED`, backoff e recuperação, preservando o eventId.
- **Referências pendentes**: PENDING_REFERENCE com worker de retry/TTL.
- **Auth**: OIDC/Keycloak, client_credentials, providerId da identidade.
- **Fx e shutdown**, observabilidade.

## Estado atual

Prontos e testados: domínio (Money, Wallet, ledger) com invariantes e overflow; migrations
com constraints validadas no banco; repositório de carteira com débito concorrente
(`FOR UPDATE` + ledger no mesmo commit), comprovado pelo teste 80/80 com `-race`.
