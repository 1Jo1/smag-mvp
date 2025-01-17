package kafka

import (
	"time"

	"github.com/codeuniversity/smag-mvp/utils"
	"github.com/segmentio/kafka-go"
)

// ReaderConfig is an internal helper structure for creating
// a new pre-configurated kafka-go Reader
type ReaderConfig struct {
	Address string
	GroupID string
	Topic   string
}

// WriterConfig is an internal helper structure for creating
// a new pre-configurated kafka-go Writer
type WriterConfig struct {
	Address string
	Topic   string
	Async   bool
}

// NewReaderConfig creates a new ReaderConfig structure
func NewReaderConfig(kafkaAddress, groupID, topic string) *ReaderConfig {
	return &ReaderConfig{
		Address: kafkaAddress,
		GroupID: groupID,
		Topic:   topic,
	}
}

// NewWriterConfig creates a new WriterConfig structure
func NewWriterConfig(kafkaAddress, topic string, async bool) *WriterConfig {
	return &WriterConfig{
		Address: kafkaAddress,
		Topic:   topic,
		Async:   async,
	}
}

// NewReader creates a kafka-go Reader structure by using common
// configuration and additionally applying the ReaderConfig on it
func NewReader(c *ReaderConfig) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{c.Address},
		GroupID:        c.GroupID,
		Topic:          c.Topic,
		MinBytes:       10e3, // 10KB
		MaxBytes:       10e6, // 10MB
		CommitInterval: time.Second,
	})
}

// NewWriter creates a kafka-go Writer structure by using common
// configurations and additionally applying the WriterConfig on it
func NewWriter(c *WriterConfig) *kafka.Writer {
	return kafka.NewWriter(kafka.WriterConfig{
		Brokers:  []string{c.Address},
		Topic:    c.Topic,
		Balancer: &kafka.LeastBytes{},
		Async:    c.Async,
	})
}

// GetInserterConfig is a convenience function for gathering the necessary
// kafka configuration for all inserters
func GetInserterConfig() (*ReaderConfig, *WriterConfig, bool) {
	var readerConfig *ReaderConfig
	var writerConfig *WriterConfig
	var wTopic string

	kafkaAddress := utils.GetStringFromEnvWithDefault("KAFKA_ADDRESS", "127.0.0.1:9092")
	isUserDiscovery := utils.GetBoolFromEnvWithDefault("USER_DISCOVERY", false)

	groupID := utils.MustGetStringFromEnv("KAFKA_GROUPID")
	rTopic := utils.MustGetStringFromEnv("KAFKA_NAME_TOPIC")

	if isUserDiscovery {
		wTopic = utils.MustGetStringFromEnv("KAFKA_INFO_TOPIC")
		writerConfig = NewWriterConfig(kafkaAddress, wTopic, true)
	}

	readerConfig = NewReaderConfig(kafkaAddress, groupID, rTopic)
	return readerConfig, writerConfig, isUserDiscovery
}

// GetScraperConfig is a convenience function for gathering the necessary
// kafka configuration for all golang scrapers
func GetScraperConfig() (*ReaderConfig, *WriterConfig, *WriterConfig) {
	var nameReaderConfig *ReaderConfig
	var infoWriterConfig *WriterConfig
	var errWriterConfig *WriterConfig

	kafkaAddress := utils.GetStringFromEnvWithDefault("KAFKA_ADDRESS", "127.0.0.1:9092")

	groupID := utils.MustGetStringFromEnv("KAFKA_GROUPID")
	nameTopic := utils.MustGetStringFromEnv("KAFKA_NAME_TOPIC")
	infoTopic := utils.MustGetStringFromEnv("KAFKA_INFO_TOPIC")
	errTopic := utils.MustGetStringFromEnv("KAFKA_ERR_TOPIC")

	nameReaderConfig = NewReaderConfig(kafkaAddress, groupID, nameTopic)
	infoWriterConfig = NewWriterConfig(kafkaAddress, infoTopic, true)
	errWriterConfig = NewWriterConfig(kafkaAddress, errTopic, false)

	return nameReaderConfig, infoWriterConfig, errWriterConfig
}
