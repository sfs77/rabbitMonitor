package main

import (
	"encoding/json"
	"fmt"
	ssh "github.com/helloyi/go-sshclient"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/streadway/amqp"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

func newGaugeVec(metricName string, docString string, labels []string) *prometheus.GaugeVec {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: PROMETHEUS_NAMESPACE,
			Name:      metricName,
			Help:      docString,
		},
		labels,
	)
}

type VoRabbitExporter struct {
	mutex         sync.RWMutex
	up            *prometheus.GaugeVec
	diskFreeAlarm *prometheus.GaugeVec
	memAlarm      *prometheus.GaugeVec
	publishMsgOk  *prometheus.GaugeVec
	ackMsgOK      *prometheus.GaugeVec
}

var (
	//prometheus 指标tags
	nodeLabels = []string{"cluster", "node"}
	//rabbitmq 测试用队列 消息超时时间 30min
	queueArgs = amqp.Table{"x-message-ttl": int32(180000)}

	allQueues []queueStatus
)

func NewVoRabbitExporter() *VoRabbitExporter {
	return &VoRabbitExporter{
		up:            newGaugeVec("up", "rabbit node is up", nodeLabels),
		diskFreeAlarm: newGaugeVec("disk_free_alarm", "alarm firing when disk free space is insufficient", nodeLabels),
		memAlarm:      newGaugeVec("mem_alarm", "alarm firing when memory usage reach high watermark", nodeLabels),
		publishMsgOk:  newGaugeVec("publish_msq_ok", "publishing message is ok", nodeLabels),
		ackMsgOK:      newGaugeVec("ack_msg_ok", "acknowledging message is ok ", nodeLabels),
	}
}

func (v *VoRabbitExporter) Describe(ch chan<- *prometheus.Desc) {
	v.up.Describe(ch)
	v.diskFreeAlarm.Describe(ch)
	v.memAlarm.Describe(ch)
	v.publishMsgOk.Describe(ch)
	v.ackMsgOK.Describe(ch)
}

func (v *VoRabbitExporter) Collect(ch chan<- prometheus.Metric) {
	v.mutex.Lock()
	defer v.mutex.Unlock()
	//如果没有获取到rabbit node status, 即调用rabbit management api 失败，则开始遍历检查节点是否存活
	if len(LATEST_RABBIT_STATUS) == 0 {
		for _, node := range strings.Split(viper.GetString("monitor_nodes_name"), ",") {
			if isUp(node) {
				v.up.WithLabelValues(DEFAULT_RABBIT_CLUSTER_NAME, "rabbit@"+node).Set(1)
			} else {
				v.up.WithLabelValues(DEFAULT_RABBIT_CLUSTER_NAME, "rabbit@"+node).Set(0)
			}
		}
	}
	//management api相应正常的情况下
	for _, ns := range LATEST_RABBIT_STATUS {
		nodeHostname := strings.Split(ns.Name, "@")[1]

		if isUp(nodeHostname) {
			v.up.WithLabelValues(DEFAULT_RABBIT_CLUSTER_NAME, ns.Name).Set(1)
		} else {
			v.up.WithLabelValues(DEFAULT_RABBIT_CLUSTER_NAME, ns.Name).Set(0)
		}

		if ns.MemAlarm {
			v.memAlarm.WithLabelValues(DEFAULT_RABBIT_CLUSTER_NAME, ns.Name).Set(1)
		} else {
			v.memAlarm.WithLabelValues(DEFAULT_RABBIT_CLUSTER_NAME, ns.Name).Set(0)
		}

		if ns.DiskFreeAlarm {
			v.diskFreeAlarm.WithLabelValues(DEFAULT_RABBIT_CLUSTER_NAME, ns.Name).Set(1)
		} else {
			v.diskFreeAlarm.WithLabelValues(DEFAULT_RABBIT_CLUSTER_NAME, ns.Name).Set(0)
		}

		if sendMsgOK(nodeHostname) {
			v.publishMsgOk.WithLabelValues(DEFAULT_RABBIT_CLUSTER_NAME, ns.Name).Set(1)
		} else {
			v.publishMsgOk.WithLabelValues(DEFAULT_RABBIT_CLUSTER_NAME, ns.Name).Set(0)
		}

		if ackMsgOK(nodeHostname) {
			v.ackMsgOK.WithLabelValues(DEFAULT_RABBIT_CLUSTER_NAME, ns.Name).Set(1)
		} else {
			v.ackMsgOK.WithLabelValues(DEFAULT_RABBIT_CLUSTER_NAME, ns.Name).Set(0)
		}
	}
	v.up.Collect(ch)
	v.diskFreeAlarm.Collect(ch)
	v.memAlarm.Collect(ch)
	v.publishMsgOk.Collect(ch)
	v.ackMsgOK.Collect(ch)
}

type nodeStatus struct {
	Name          string `json:"name"`
	FdUsed        int64  `json:"fd_used"`
	FdTotal       int64  `json:"fd_total"`
	SocketsUsed   int64  `json:"sockets_used"`
	SocketsTotal  int64  `json:"sockets_total"`
	MemUsed       int64  `json:"mem_used"`
	MemLimit      int64  `json:"mem_limit"`
	MemAlarm      bool   `json:"mem_alarm"`
	DiskFreeLimit int64  `json:"disk_free_limit"`
	DiskFree      int64  `json:"disk_free"`
	DiskFreeAlarm bool   `json:"disk_free_alarm"`
	Running       bool   `json:"running"`
}

type queueStatus struct {
	Name         string       `json:"name"`
	Messages     int64        `json:"messages"`
	MessageStats messageStats `json:"message_stats"`
	State        string       `json:"state"`
	Durable      bool         `json:"durable"`
	AutoDelete   bool         `json:"auto_delete"`
}

type messageStats struct {
	AckDetails     ackDetails     `json:"ack_details"`
	PublishDetails publishDetails `json:"publish_details"`
}

type ackDetails struct {
	Rate float64 `json:"rate"`
}

type publishDetails struct {
	Rate float64 `json:"rate"`
}

func getRabbitStatus(nodeIp string) (err error) {
	req, err := http.NewRequest(http.MethodGet,
		fmt.Sprintf("http://%s:%s/api/nodes",
			nodeIp, viper.GetString("rabbit_management_port")),
		nil)
	req.SetBasicAuth(viper.GetString("rabbit_user"), viper.GetString("rabbit_password"))
	resp, err := HTTP_CLIENT.Do(req)
	if err != nil {
		log.Errorf("request rabbit management api failed: %#v", err)
		return
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	json.Unmarshal(body, &LATEST_RABBIT_STATUS)
	return
}

func getQueuesStatus(nodeIp string) {
	req, _ := http.NewRequest(http.MethodGet,
		fmt.Sprintf("http://%s:%s/api/queues",
			nodeIp, viper.GetString("rabbit_management_port")),
		nil)
	req.SetBasicAuth(viper.GetString("rabbit_user"), viper.GetString("rabbit_password"))
	resp, err := HTTP_CLIENT.Do(req)
	if err != nil {
		log.Errorf("request rabbit management api failed: %#v \n", err)
		return
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error(err)
		return
	}
	json.Unmarshal(body, &allQueues)
}

//ssh返回值
type sshReturn struct {
	output string
	err    error
}

//ssh控制函数
func ctlRabbit(nodeIp string, action string) (string, error) {
	SSH_RWMUTEX.Lock() //ssh执行锁，不允许同时执行
	defer SSH_RWMUTEX.Unlock()
	var command string
	switch action {
	case "start":
		command = "start"
	case "stop":
		command = "stop"
	case "restart":
		command = "restart"
	default:
		return "", MyError("invalid rabbitmq shell command verb")
	}
	cmdReturnCh := make(chan sshReturn)
	//goroutine后台执行ssh, 前台等待
	go func() {
		sshClient, err := ssh.DialWithPasswd(nodeIp+":22", viper.GetString("ssh_user"), viper.GetString("ssh_password"))
		if err != nil {
			cmdReturnCh <- sshReturn{
				output: "",
				err:    err,
			}
			return
		}
		defer sshClient.Close()
		cmdOutputBytes, err := sshClient.Cmd(viper.GetString("rabbit_cli") + " " + command).SmartOutput()
		cmdReturnCh <- sshReturn{
			output: string(cmdOutputBytes),
			err:    err,
		}
		return
	}()
	//ssh执行超时时间1min, 超时后提示并返回
	select {
	case cmdReturn := <-cmdReturnCh:
		log.Infof(`execute rabbitctl command %s, command stdout: %s, command err: %s`, command, cmdReturn.output, cmdReturn.err)
		return cmdReturn.output, cmdReturn.err
	case <-time.After(time.Minute * 1):
		log.Errorln(`execute rabbitctl command "` + command + `" timeout， please manual check!`)
		return "", MyError("ssh execute command timeout")
	}
}

func generateTop10MostMsgsQueues() {
	sort.SliceStable(allQueues, func(i, j int) bool {
		return allQueues[i].Messages > allQueues[j].Messages
	})
	realLen := 10
	if len(allQueues) < 10 {
		realLen = len(allQueues)
	}
	var resultSlices = make([]queueStatus, realLen)
	copy(resultSlices, allQueues)
	TOP10_MOST_MSGS_QUEUES = resultSlices
}

func generateTop10PublishRateQueues() {
	sort.SliceStable(allQueues, func(i, j int) bool {
		return allQueues[i].MessageStats.PublishDetails.Rate > allQueues[j].MessageStats.PublishDetails.Rate
	})
	realLen := 10
	if len(allQueues) < 10 {
		realLen = len(allQueues)
	}
	var resultSlices = make([]queueStatus, realLen)
	copy(resultSlices, allQueues)
	TOP10_PUBLISH_RATE_QUEUES = resultSlices
}

func generateTop10AckRateQueues() {
	sort.SliceStable(allQueues, func(i, j int) bool {
		return allQueues[i].MessageStats.AckDetails.Rate > allQueues[j].MessageStats.AckDetails.Rate
	})
	realLen := 10
	if len(allQueues) < 10 {
		realLen = len(allQueues)
	}
	var resultSlices = make([]queueStatus, realLen)
	copy(resultSlices, allQueues)
	TOP10_ACK_RATE_QUEUES = resultSlices
}

func getQueuesOverview(nodeIp string) {
	getQueuesStatus(nodeIp)
	generateTop10MostMsgsQueues()
	generateTop10PublishRateQueues()
	generateTop10AckRateQueues()
}

func isUp(nodeIp string) bool {
	req, _ := http.NewRequest(http.MethodGet,
		fmt.Sprintf("http://%s:%s/",
			nodeIp, viper.GetString("rabbit_management_port")),
		nil)
	req.SetBasicAuth(viper.GetString("rabbit_user"), viper.GetString("rabbit_password"))
	resp, err := HTTP_CLIENT.Do(req)
	if err != nil {
		log.Errorf("request rabbit management endpoint failed: %#v \n", err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 && resp.StatusCode >= 300 {
		log.Warnf("request rabbit management api get bad status code, node [%s] may failed \n", nodeIp)
		return false
	} else {
		return true
	}
}

func deleteQueue(nodeIp string, queueName string) error {
	req, _ := http.NewRequest(http.MethodDelete,
		fmt.Sprintf("http://%s:%s/api/queues/%%2F/%s",
			nodeIp, viper.GetString("rabbit_management_port"), queueName),
		nil)
	req.SetBasicAuth(viper.GetString("rabbit_user"), viper.GetString("rabbit_password"))
	_, err := HTTP_CLIENT.Do(req)
	if err != nil {
		log.Errorf("request rabbit management api failed: %#v \n", err)
	}
	return err
}

func sendMsgOK(nodeIp string) bool {
	//conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	conn, err := amqp.Dial(
		fmt.Sprintf("amqp://%s:%s@%s:%s/%%2F",
			viper.GetString("rabbit_user"),
			viper.GetString("rabbit_password"),
			nodeIp,
			viper.GetString("rabbit_amqp_port")))
	if err != nil {
		log.Println("Failed to connect to RabbitMQ Node")
		return false
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Println("Failed to open a channel")
		return false
	}
	defer ch.Close()

	// only fetch one message
	ch.Qos(1, 0, false)

	q, err := ch.QueueDeclare(
		viper.GetString("monitor_queue_name"), // name
		true,                                  // durable
		false,                                 // delete when unused
		false,                                 // exclusive
		false,                                 // no-wait
		queueArgs,                             // arguments
	)
	if err != nil {
		log.Println("Failed to declare a queue")
		return false
	}

	body := "VO Rabbitmq Monitor Test Message at " + time.Now().String()
	err = ch.Publish(
		"",     // exchange
		q.Name, // routing key
		false,  // mandatory
		false,  // immediate
		amqp.Publishing{
			ContentType:  "text/plain",
			Body:         []byte(body),
			DeliveryMode: amqp.Persistent,
		})
	log.Printf(" [x] Sent %s \n", body)
	if err != nil {
		log.Println("Failed to publish a message")
		return false
	}
	return true
}

func ackMsgOK(nodeIp string) bool {
	//conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	conn, err := amqp.Dial(
		fmt.Sprintf("amqp://%s:%s@%s:%s/%%2F",
			viper.GetString("rabbit_user"),
			viper.GetString("rabbit_password"),
			nodeIp,
			viper.GetString("rabbit_amqp_port")))
	if err != nil {
		log.Println("Failed to connect to RabbitMQ Node")
		return false
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Println("Failed to open a channel")
		return false
	}
	// only fetch one message
	ch.Qos(1, 0, false)
	defer ch.Close()

	q, err := ch.QueueDeclare(
		viper.GetString("monitor_queue_name"), // name
		true,                                  // durable
		false,                                 // delete when unused
		false,                                 // exclusive
		false,                                 // no-wait
		queueArgs,                             // arguments
	)
	if err != nil {
		log.Println("Failed to declare a queue")
		return false
	}

	msg, ok, err := ch.Get(
		q.Name, // queue
		true,   // auto-ack
	)
	if err != nil {
		log.Println("Failed to register a consumer")
		return false
	}
	if ok {
		log.Printf("Received a message: %s\n", msg.Body)
	} else {
		log.Printf("Queue %s is empty\n", q.Name)
	}
	return true
}
