package pulsar

import (
	"context"
	"log"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
)

// Client is a puslar client wrapper of the official pulsar.Client
type Client struct {
	pulsar.Client
	producers map[string]Producer
}

// NewClient create a pulsar client with url and token
func NewClient(url string, token string) (*Client, error) {
	var err error
	option := pulsar.ClientOptions{}
	if token == "" {
		option = pulsar.ClientOptions{
			URL:               url,
			OperationTimeout:  30 * time.Second,
			ConnectionTimeout: 30 * time.Second,
		}
	} else {
		option = pulsar.ClientOptions{
			URL:               url,
			OperationTimeout:  30 * time.Second,
			ConnectionTimeout: 30 * time.Second,
			Authentication:    pulsar.NewAuthenticationToken(token),
		}
	}
	client, err := pulsar.NewClient(option)
	if err != nil {
		log.Fatalf("Could not instantiate Pulsar client: %v, url: %s, token : %s", err, url, token)
		return nil, err
	}
	return &Client{
		Client:    client,
		producers: make(map[string]pulsar.Producer),
	}, nil
}

// GetOrCreateProducer create a pulsar producer
func (c *Client) GetOrCreateProducer(topic string) (Producer, error) {
	var err error
	producer, ok := c.producers[topic]
	if !ok {
		producer, err = c.Client.CreateProducer(pulsar.ProducerOptions{
			Topic: topic,
		})
		if err != nil {
			log.Fatalf("Could not instantiate Pulsar producer: %v", err)
			return nil, err
		}
		c.producers[topic] = producer
	}
	return producer, nil
}

// Close closes producers and client
func (c *Client) Close() {
	for _, p := range c.producers {
		p.Close()
	}
	c.Client.Close()
}

// SendMessages send fluent-bit log as messages to pulsar
func (c *Client) SendMessages(messages map[string][]string) {
	var producer pulsar.Producer
	var err error
	for topic, msgs := range messages {
		producer, err = c.GetOrCreateProducer(topic)
		if err != nil {
			log.Printf("Can not get producer for topic %s", topic)
			continue
		}
		for _, msg := range msgs {
			producer.SendAsync(context.Background(), &pulsar.ProducerMessage{
				Payload: []byte(msg),
			}, func(id pulsar.MessageID, producerMessage *pulsar.ProducerMessage, e error) {
				if e != nil {
					log.Printf("Failed to publish message %v, error %v\n", producerMessage, e)
				}
			})
		}
		if err = producer.Flush(); err != nil {
			log.Printf("Failed to Flush, error %v\n", err)
		}
	}
}

// ConsumerMessage is the pulsar consumer message type
type ConsumerMessage = pulsar.ConsumerMessage

// MessageChannel is the pulsar consumer message channel
type MessageChannel = chan ConsumerMessage

// Consumer is the pulsar consumer
type Consumer = pulsar.Consumer

// Producer is the pulsar producer
type Producer = pulsar.Producer

// Message is the pulsar messages
type Message = pulsar.Message
