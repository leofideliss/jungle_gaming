#!/bin/bash
awslocal sqs create-queue \
  --queue-name wager-transactions.fifo \
  --attributes '{
    "FifoQueue": "true",
    "ContentBasedDeduplication": "false",
    "VisibilityTimeout": "30",
    "MessageRetentionPeriod": "86400"
  }'

echo "Fila criada: wager-transactions.fifo"
