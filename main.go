package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/pkg/errors"

	log "github.com/sirupsen/logrus"
)

var appLogger *log.Entry

const APP_NAME = "PoESocial"
const BROKER = "localhost:9092"
const DELEGATOR = "http://localhost:8080"
const WRITE_PROXY_LISTEN = ":9064"

func main() {
	routes := make(map[string]string)
	// should support multiple app in one proxy
	routes["PoESocial_stat:UPDATE"] = fmt.Sprintf("%s/%s", DELEGATOR, "get_stashes")

	topic := fmt.Sprintf("^%s_*", APP_NAME)

	appLogger = log.WithFields(log.Fields{"app_name": APP_NAME, "broker": BROKER})

	// create a producer
	producer, err := kafka.NewProducer(&kafka.ConfigMap{"bootstrap.servers": BROKER})
	if err != nil {
		appLogger.Fatal("Unable to create producer:", err)
	}

	// generic logging for producer
	go func() {
		for e := range producer.Events() {
			switch ev := e.(type) {
			case *kafka.Message:
				m := ev
				if m.TopicPartition.Error != nil {
					appLogger.Fatal("delivery failed", m.TopicPartition.Error)
				} else {
					appLogger.Infof("Delivered message to topic %s [%d] at offset %v\n",
						*m.TopicPartition.Topic, m.TopicPartition.Partition, m.TopicPartition.Offset)
				}
				return
			default:
				appLogger.Info("Ignored event: %s\n", ev)
			}
		}
	}()

	// create a proxy to let functions write data back to kafka
	writeProxy, err := NewWriteProxy(producer.ProduceChannel())
	if err != nil {
		appLogger.Fatal(err)
	}
	go http.ListenAndServe(WRITE_PROXY_LISTEN, writeProxy)

	consumer, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":               BROKER,
		"group.id":                        fmt.Sprintf("spacer-router-%s", APP_NAME),
		"session.timeout.ms":              6000,
		"go.application.rebalance.enable": true,
		"default.topic.config":            kafka.ConfigMap{"auto.offset.reset": "earliest"},
		"metadata.max.age.ms":             1000,
		"enable.auto.commit":              false,
	})
	if err != nil {
		appLogger.Fatal("Failed to create consumer", err)
	}
	defer consumer.Close()

	err = consumer.SubscribeTopics([]string{topic}, nil)
	if err != nil {
		appLogger.Fatal("Failed to subscribe topics %v, %s\n", topic, err)
	}

	// periodically refresh metadata to know if there's any new topic created
	go refreshMetadata(consumer, appLogger)

	// start the consumer loop
	for {
		ev := consumer.Poll(100)
		if ev == nil {
			continue
		}
		switch e := ev.(type) {
		case kafka.AssignedPartitions:
			appLogger.Info(e)
			consumer.Assign(e.Partitions)
		case kafka.RevokedPartitions:
			appLogger.Info(e)
			consumer.Unassign()
		case *kafka.Message:
			appLogger.Info("%% Message on ", e.TopicPartition)

			routePath := fmt.Sprintf("%s:UPDATE", *e.TopicPartition.Topic)
			appLogger.Info("Looking up route ", routePath)

			if _, ok := routes[routePath]; !ok {
				appLogger.Info("Route not found")
				continue
			}

			err := invoke(routes[routePath], []byte(string(e.Value)))
			if err != nil {
				appLogger.Error("Invocation Error: %v\n", err)
				continue
			}
			_, err = consumer.CommitMessage(e)
			if err != nil {
				appLogger.Error("Commit Error: %v %v\n", e, err)
			}
		case kafka.PartitionEOF:
			appLogger.Info("%% Reached", e)
		case kafka.Error:
			appLogger.Fatal("%% Error", e)
		default:
			appLogger.Info("Unknown", e)
		}
	}
}

func invoke(route string, data []byte) error {
	appLogger.Infof("Invoking %s\n", route)
	resp, err := http.Post(route, "application/json", bytes.NewReader(data))

	if err != nil {
		return errors.Wrap(err, "post event handler failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return errors.New(fmt.Sprintf("Function not ok: %d", resp.StatusCode))

	}
	return nil
}

type WriteProxy struct {
	produceChan chan *kafka.Message
}

func NewWriteProxy(produceChan chan *kafka.Message) (*WriteProxy, error) {
	proxy := WriteProxy{produceChan}
	return &proxy, nil
}

func (p WriteProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		appLogger.Error("read body failed", err)
		w.WriteHeader(400)
		return
	}
	var write WriteRequest
	err = json.Unmarshal(body, &write)
	if err != nil {
		appLogger.Errorf("decode body failed %v\n", err)
		w.WriteHeader(400)
		return
	}
	topic := fmt.Sprintf("%s_%s", APP_NAME, write.Object)
	for key, value := range write.Data {
		fmt.Println("Writing", write.Object, key)
		p.produceChan <- &kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
			Key:            []byte(key),
			Value:          value,
		}
	}
	fmt.Fprintf(w, "ok")
}

type WriteRequest struct {
	Object string                     `json:"object"`
	Data   map[string]json.RawMessage `json:"data"`
}

func refreshMetadata(consumer *kafka.Consumer, logger *log.Entry) {
	for {
		metadata, err := consumer.GetMetadata(nil, true, 100)
		if err != nil {
			// somethimes it just timed out, ignore
			logger.Warn("Unable to refresh metadata: ", err)
			continue
		}
		keys := []string{}
		for k, _ := range metadata.Topics {
			keys = append(keys, k)
		}
		// logger.Info("metadata: ", keys)
		time.Sleep(5 * time.Second)
	}
}
