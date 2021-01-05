package main

import (
	"C"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
	"unsafe"

	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/fluent/fluent-bit-go/output"
)

var (
	producer      pulsar.Producer
	configContext map[string]string
	client        pulsar.Client
	producers     map[string]pulsar.Producer
)

func init() {
	configContext = make(map[string]string)
	producers = make(map[string]pulsar.Producer)
}

func initPulsarClient(url string, token string) error {
	var err error
	client, err = pulsar.NewClient(pulsar.ClientOptions{
		URL:               url,
		OperationTimeout:  30 * time.Second,
		ConnectionTimeout: 30 * time.Second,
		Authentication:    pulsar.NewAuthenticationToken(token),
	})
	if err != nil {
		log.Fatalf("Could not instantiate Pulsar client: %v", err)
	}
	return err
}

func getProducer(topic string) (pulsar.Producer, error) {
	var err error
	producer, ok := producers[topic]
	if !ok {
		producer, err = client.CreateProducer(pulsar.ProducerOptions{
			Topic: topic,
		})
		if err != nil {
			log.Fatalf("Could not instantiate Pulsar producer: %v", err)
		}
		producers[topic] = producer
	}
	return producer, nil
}

//export FLBPluginRegister
func FLBPluginRegister(def unsafe.Pointer) int {
	return output.FLBPluginRegister(def, "pulsar", "Pulsar GO!")
}

//export FLBPluginInit
// (fluentbit will call this)
// plugin (context) pointer to fluentbit context (state/ c code)
func FLBPluginInit(plugin unsafe.Pointer) int {
	// Example to retrieve an optional configuration parameter
	url := output.FLBPluginConfigKey(plugin, "url")
	token := output.FLBPluginConfigKey(plugin, "token")
	tenant := output.FLBPluginConfigKey(plugin, "tenant")
	namespace := output.FLBPluginConfigKey(plugin, "namespace")

	// Set the context to point to any Go variable
	configContext["url"] = url
	configContext["tenant"] = tenant
	configContext["namespace"] = namespace

	if err := initPulsarClient(url, token); err != nil {
		log.Fatalf("init pulsar client error")
	}

	output.FLBPluginSetContext(plugin, configContext)
	return output.FLB_OK
}

func parseK8sNamespaceFromTag(tag string) string {
	splits := strings.Split(tag, ".")
	if len(splits) < 2 {
		return ""
	}
	return splits[1]
}

func buildJsonBytes(record map[interface{}]interface{}) ([]byte, error) {
	m := make(map[string]string)
	for k, v := range record {
		key := fmt.Sprintf("%s", k)
		value := fmt.Sprintf("%s", v)
		m[key] = value
	}

	jsonBytes, err := json.Marshal(m)
	if err != nil {
		log.Fatalf("json.Marshal record error : %v", err)
		return nil, err
	}
	return jsonBytes, nil
}

func addMessage(m map[string][]string, key string, value string) {
	if _, ok := m[key]; !ok {
		m[key] = make([]string, 3)
	}
	m[key] = append(m[key], value)
}

func sendMessages(messages map[string][]string) {
	var producer pulsar.Producer
	var err error
	for topic, msgs := range messages {
		producer, err = getProducer(topic)
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

//export FLBPluginFlush
func FLBPluginFlush(data unsafe.Pointer, length C.int, tag *C.char) int {
	log.Print("[fluent-go] Flush called for unknown instance")
	return output.FLB_OK
}

//export FLBPluginFlushCtx
func FLBPluginFlushCtx(ctx, data unsafe.Pointer, length C.int, tag *C.char) int {
	// Type assert context back into the original type for the Go variable
	cfgContext := output.FLBPluginGetContext(ctx).(map[string]string)
	log.Printf("[fluent-go] Flush called for cfgContext: %v", cfgContext)
	tenant := cfgContext["tenant"]
	namespace := cfgContext["namespace"]

	dec := output.NewDecoder(data, int(length))

	messages := make(map[string][]string)

	count := 0
	for {
		ret, ts, record := output.GetRecord(dec)
		if ret != 0 {
			break
		}

		var timestamp time.Time
		switch t := ts.(type) {
		case output.FLBTime:
			timestamp = ts.(output.FLBTime).Time
		case uint64:
			timestamp = time.Unix(int64(t), 0)
		default:
			fmt.Println("time provided invalid, defaulting to now.")
			timestamp = time.Now()
		}

		// Print record keys and values
		fmt.Printf("[%d] %s: [%s, {", count, C.GoString(tag), timestamp.String())

		fbTag := fmt.Sprintf("%s", C.GoString(tag))
		k8sNamespace := parseK8sNamespaceFromTag(fbTag)
		topic := fmt.Sprintf("%s/%s/%s", tenant, namespace, k8sNamespace)

		for k, v := range record {
			fmt.Printf("\"%s\": %s, ", k, v)
		}
		fmt.Printf("}\n")
		count++

		m := record2map(buildMapFromRecord(record))
		jsonBytes, err := json.Marshal(m)
		if err != nil {
			log.Fatalf("json.Marshal record error : %v", err)
			continue
		}

		addMessage(messages, topic, string(jsonBytes))
	}

	// send messages in batch
	sendMessages(messages)

	return output.FLB_OK
}

func buildMapFromRecord(record map[interface{}]interface{}) map[string]interface{} {
	m := make(map[string]interface{})
	for k, v := range record {
		m[fmt.Sprintf("%s", k)] = v
	}
	return m
}

func flatten(m map[string]interface{}) map[string]interface{} {
	o := make(map[string]interface{})
	for k, v := range m {
		switch child := v.(type) {
		case map[string]interface{}:
			nm := flatten(child)
			for nk, nv := range nm {
				o[k+"."+nk] = nv
			}
		default:
			o[k] = v
		}
	}
	return o
}

func record2map(record map[string]interface{}) map[string]interface{} {
	v, ok := record["log"]
	if !ok {
		// log is not in record, return flatten record
		return flatten(record)
	}
	// try to unmarshal log's value
	m := make(map[string]interface{})
	if err := json.Unmarshal([]byte(v.(string)), &m); err != nil {
		// something wrong happens, do not unmarshal
		return flatten(record)
	}
	// we can unmarshal log's value into map
	m, err := data2map([]byte(v.(string)))
	if err != nil {
		// cannot parse log's value to map, use raw
	} else {
		// add new unmasharled map to result map, and keep the log(raw data)
		for k, v := range m {
			record[k] = v
		}
	}
	return flatten(record)
}

func data2map(data []byte) (map[string]interface{}, error) {
	m := make(map[string]interface{})
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

//export FLBPluginExit
func FLBPluginExit() int {
	return output.FLB_OK
}

func main() {
}
