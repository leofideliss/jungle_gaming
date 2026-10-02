package worker

import (
	"context"
	"jungle_gaming/internal/repository"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SQSOutboxProducer struct {
	client   *sqs.Client
	queueUrl string
	outbox   *repository.OutboxRepository
	pool     *pgxpool.Pool
}

func NewSQSOutboxProducer(queueUrl, endpoint string, outbox *repository.OutboxRepository, pool *pgxpool.Pool) (*SQSOutboxProducer, error) {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	if err != nil {
		return nil, err
	}

	client := sqs.NewFromConfig(cfg, func(o *sqs.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})

	return &SQSOutboxProducer{
		client:   client,
		queueUrl: queueUrl,
		outbox:   outbox,
		pool:     pool,
	}, nil
}

func (s *SQSOutboxProducer) Start(ctx context.Context) {
	log.Println("SQS outbox producer iniciado")
	for {
		select {
		case <-ctx.Done():
			log.Println("SQS outbox producer parando")
			return
		default:
		}

		s.publishBatch(ctx)
		time.Sleep(10 * time.Second)
	}
}

func (s *SQSOutboxProducer) publishBatch(ctx context.Context) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)
	rows, err := s.outbox.FetchPending(ctx, tx, 10)
	if err != nil {
		log.Println("Erro ao consultar registros")
		time.Sleep(5 * time.Second)
		return
	}

	if len(rows) == 0 {
		log.Println("Nada para processar")
		time.Sleep(5 * time.Second)
		return
	}

	for _, msg := range rows {
		payload := string(msg.Payload)
		input := &sqs.SendMessageInput{
			QueueUrl:               &s.queueUrl,
			MessageBody:            &payload,
			MessageGroupId:         aws.String(msg.AggregateID.String()),
			MessageDeduplicationId: aws.String(msg.EventID.String()),
		}

		_, err := s.client.SendMessage(ctx, input)
		if err != nil {
			if err := s.outbox.MarkAsFailed(ctx, tx, msg.ID); err != nil {
				log.Printf("erro ao atualizar %v", err)
				return
			}

			log.Printf("erro ao enviar mensagem: %v", err)
			return
		}

		if err := s.outbox.MarkAsPublished(ctx, tx, msg.ID); err != nil {
			log.Printf("erro ao atualizar: %v", err)
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return
	}

}
