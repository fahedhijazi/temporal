// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package messaging

import (
	"errors"
	"fmt"

	"github.com/Shopify/sarama"
	"github.com/gogo/protobuf/proto"

	indexergenpb "github.com/temporalio/temporal/.gen/proto/indexer"
	replicationgenpb "github.com/temporalio/temporal/.gen/proto/replication"
	"github.com/temporalio/temporal/common/log"
	"github.com/temporalio/temporal/common/log/tag"
)

type (
	kafkaProducer struct {
		topic    string
		producer sarama.SyncProducer
		logger   log.Logger
	}
)

var _ Producer = (*kafkaProducer)(nil)

// NewKafkaProducer is used to create the Kafka based producer implementation
func NewKafkaProducer(topic string, producer sarama.SyncProducer, logger log.Logger) Producer {
	return &kafkaProducer{
		topic:    topic,
		producer: producer,
		logger:   logger.WithTags(tag.KafkaTopicName(topic)),
	}
}

// Publish is used to send messages to other clusters through Kafka topic
func (p *kafkaProducer) Publish(msg interface{}) error {
	message, err := p.getProducerMessage(msg)
	if err != nil {
		return err
	}

	partition, offset, err := p.producer.SendMessage(message)
	if err != nil {
		p.logger.Warn("Failed to publish message to kafka",
			tag.KafkaPartition(partition),
			tag.KafkaPartitionKey(message.Key),
			tag.KafkaOffset(offset),
			tag.Error(err))
		return p.convertErr(err)
	}

	return nil
}

// Close is used to close Kafka publisher
func (p *kafkaProducer) Close() error {
	return p.convertErr(p.producer.Close())
}

func (p *kafkaProducer) serializeProto(input proto.Marshaler) ([]byte, error) {
	payload, err := input.Marshal()
	if err != nil {
		p.logger.Error("Failed to serialize proto object", tag.Error(err))

		return nil, err
	}

	return payload, nil
}

func (p *kafkaProducer) getKeyForReplicationTask(task *replicationgenpb.ReplicationTask) sarama.Encoder {
	if task == nil {
		return nil
	}

	switch task.GetTaskType() {
	case replicationgenpb.ReplicationTaskTypeHistory:
		// Use workflowID as the partition key so all replication tasks for a workflow are dispatched to the same
		// Kafka partition.  This will give us some ordering guarantee for workflow replication tasks at least at
		// the messaging layer perspective
		attributes := task.GetHistoryTaskAttributes()
		return sarama.StringEncoder(attributes.GetWorkflowId())
	case replicationgenpb.ReplicationTaskTypeHistoryV2:
		// Use workflowID as the partition key so all replication tasks for a workflow are dispatched to the same
		// Kafka partition.  This will give us some ordering guarantee for workflow replication tasks at least at
		// the messaging layer perspective
		attributes := task.GetHistoryTaskV2Attributes()
		return sarama.StringEncoder(attributes.GetWorkflowId())
	case replicationgenpb.ReplicationTaskTypeSyncActivity:
		// Use workflowID as the partition key so all sync activity tasks for a workflow are dispatched to the same
		// Kafka partition.  This will give us some ordering guarantee for workflow replication tasks atleast at
		// the messaging layer perspective
		attributes := task.GetSyncActivityTaskAttributes()
		return sarama.StringEncoder(attributes.GetWorkflowId())
	case replicationgenpb.ReplicationTaskTypeHistoryMetadata,
		replicationgenpb.ReplicationTaskTypeNamespace,
		replicationgenpb.ReplicationTaskTypeSyncShardStatus:
		return nil
	default:
		panic(fmt.Sprintf("encounter unsupported replication task type: %v", task.GetTaskType()))
	}

	return nil
}

func (p *kafkaProducer) getProducerMessage(message interface{}) (*sarama.ProducerMessage, error) {
	switch message := message.(type) {
	case *replicationgenpb.ReplicationTask:
		payload, err := p.serializeProto(message)
		if err != nil {
			return nil, err
		}
		partitionKey := p.getKeyForReplicationTask(message)
		msg := &sarama.ProducerMessage{
			Topic: p.topic,
			Key:   partitionKey,
			Value: sarama.ByteEncoder(payload),
		}
		return msg, nil
	case *indexergenpb.Message:
		payload, err := p.serializeProto(message)
		if err != nil {
			return nil, err
		}
		msg := &sarama.ProducerMessage{
			Topic: p.topic,
			Key:   sarama.StringEncoder(message.GetWorkflowId()),
			Value: sarama.ByteEncoder(payload),
		}
		return msg, nil
	default:
		return nil, errors.New("unknown producer message type")
	}
}

func (p *kafkaProducer) convertErr(err error) error {
	switch err {
	case sarama.ErrMessageSizeTooLarge:
		return ErrMessageSizeLimit
	default:
		return err
	}
}
