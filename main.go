package main

import (
	"C"
	"encoding/json"
	"fmt"
	"log"
	"unsafe"

	"github.com/fluent/fluent-bit-go/output"
	"github.com/lichuan6/fluent-bit-pulsar-go/pulsar"
	"github.com/lichuan6/fluent-bit-pulsar-go/util"
)

var (
	configContext map[string]string
	client        *pulsar.Client
)

func init() {
	configContext = make(map[string]string)
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
	debug := output.FLBPluginConfigKey(plugin, "debug")

	log.Printf("[FLBPluginInit]url: %s, token: %s, tenant: %s, namespace: %s\n", url, token, tenant, namespace)

	// Set the context to point to any Go variable
	configContext["url"] = url
	configContext["tenant"] = tenant
	configContext["namespace"] = namespace
	configContext["debug"] = debug

	var err error
	client, err = pulsar.NewClient(url, token)
	if err != nil {
		log.Fatalf("init pulsar client error: %v", err)
	}

	output.FLBPluginSetContext(plugin, configContext)
	return output.FLB_OK
}

//export FLBPluginFlush
func FLBPluginFlush(data unsafe.Pointer, length C.int, tag *C.char) int {
	return output.FLB_OK
}

//export FLBPluginFlushCtx
func FLBPluginFlushCtx(ctx, data unsafe.Pointer, length C.int, tag *C.char) int {
	// Type assert context back into the original type for the Go variable
	cfgContext := output.FLBPluginGetContext(ctx).(map[string]string)
	tenant := cfgContext["tenant"]
	namespace := cfgContext["namespace"]
	debug := configContext["debug"]

	dec := output.NewDecoder(data, int(length))

	messages := make(map[string][]string)

	for {
		ret, _, record := output.GetRecord(dec)
		if ret != 0 {
			break
		}

		fbTag := fmt.Sprintf("%s", C.GoString(tag))
		k8sNamespace := util.ParseK8sNamespaceFromTag(fbTag)
		topic := fmt.Sprintf("%s/%s/%s", tenant, namespace, k8sNamespace)

		if debug == "true" {
			for k, v := range record {
				fmt.Printf("tag: %s, k8s ns: %s, topic: %s, record: \"%s\": %v, \n", fbTag, k8sNamespace, topic, k, v)
			}
		}

		// the type of record is map[interface{}]interface{}
		// in order to serialize and send to pulsar
		// we need to convert it to map[string]interface{}
		recordConverted := util.Convert(record)
		m := util.FlattenRecordMap(recordConverted)
		jsonBytes, err := json.Marshal(m)
		if err != nil {
			log.Fatalf("json.Marshal record error : %v", err)
			continue
		}

		util.AddMessage(messages, topic, string(jsonBytes))
	}

	// send messages in batch
	client.SendMessages(messages)

	return output.FLB_OK
}

//export FLBPluginExit
func FLBPluginExit() int {
	client.Close()
	log.Println("puslar plugin exit")
	return output.FLB_OK
}

func main() {
}
