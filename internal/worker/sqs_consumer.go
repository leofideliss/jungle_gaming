package worker

import (
	"context"
	"encoding/json"
	"jungle_gaming/internal/usecase"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

type SQSConsumer struct {
	client   *sqs.Client
	queueUrl string
	useCase  *usecase.WagerUseCase
}

func NewSQSConsumer(queueURL, endpoint string, uc *usecase.WagerUseCase) (*SQSConsumer, error) {
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

	return &SQSConsumer{
		client:   client,
		queueUrl: queueURL,
		useCase:  uc,
	}, nil
}

func (s *SQSConsumer) Start(ctx context.Context) {
	log.Println("SQS consumer iniciado")

	for {
		select {
		case <-ctx.Done():
			log.Println("SQS consumer parando")
			return
		default:
		}

		output, err := s.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            &s.queueUrl,
			MaxNumberOfMessages: 10,
			WaitTimeSeconds:     20,
		})
		if err != nil {
			log.Printf("error ao receber mensagem: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		for _, msg := range output.Messages {
			s.processMessage(ctx, msg)
		}
	}
}

func (s *SQSConsumer) processMessage(ctx context.Context, msg types.Message) {
	var request usecase.WagerRequestDTO
	if err := json.Unmarshal([]byte(*msg.Body), &request); err != nil {
		log.Printf("mensagem invalida descartando %v", err)
		s.deleteMessage(ctx, msg)
		return
	}

	in, err := request.ToInput()
	if err != nil {
		log.Printf("dados invalidos %v", err)
		s.deleteMessage(ctx, msg)

		return
	}

	_, err = s.useCase.ProcessWagerTransaction(in)
	if err != nil {
		log.Printf("erro ao processar %v", err)
		return
	}

	s.deleteMessage(ctx, msg)
}

func (s *SQSConsumer) deleteMessage(ctx context.Context, msg types.Message) {
	_, err := s.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      &s.queueUrl,
		ReceiptHandle: msg.ReceiptHandle,
	})

	if err != nil {
		log.Printf("erro ao deletar mensagem %v", err)
	}
}
