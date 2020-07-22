package main

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/viper"
	"path/filepath"
	"testing"
)

func TestGetRabbitStatus(t *testing.T) {
	err := getRabbitStatus(viper.GetString("rabbit_host"))
	j, _ := json.Marshal(LATEST_RABBIT_STATUS)
	fmt.Println(string(j))
	if err != nil {
		t.Error(err)
	}
}

func TestGenerateTop10MostMsgsQueues(t *testing.T) {
	getQueuesStatus(viper.GetString("rabbit_host"))
	generateTop10MostMsgsQueues()
	fmt.Println(TOP10_MOST_MSGS_QUEUES)
}

func TestSendMsgOK(t *testing.T) {
	if !sendMsgOK("10.0.1.208") {
		t.Fail()
	}
}

func TestAckMsgOk(t *testing.T) {
	if !ackMsgOK("10.0.1.208") {
		t.Fail()
	}
}

func TestDeleteQueue(t *testing.T) {
	err := deleteQueue("rabbit", viper.GetString("monitor_queue_name"))
	if err != nil {
		t.Error(err)
	}
}

func TestStopRabbit(t *testing.T) {
	output, err := ctlRabbit("rabbit", "stop")
	if err != nil {
		t.Error(err)
	} else {
		print(output)
	}
}

func TestStartRabbit(t *testing.T) {
	output, err := ctlRabbit("rabbit", "start")
	if err != nil {
		t.Error(err)
	} else {
		print(output)
	}
}

func TestFilePathFunc(t *testing.T) {
	fmt.Println(filepath.Dir(""))
	fmt.Println(filepath.Base(""))
}
