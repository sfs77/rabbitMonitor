package main

import (
	"fmt"
	"github.com/CatchZeng/dingtalk/client"
	"github.com/CatchZeng/dingtalk/message"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"strings"
)

func generateLevelMsg(level string) (msg string) {
	switch level {
	case "WARN":
		msg = "⛈WARN"
	case "FATAL":
		msg = "🔥FATAL"
	case "FORCE_RESTART":
		msg = "⚡FORCE_RESTART"
	default:
		msg = "💬INFO"
	}
	return
}

func generateDingTitle(a *Alert) (title string) {
	title = `ERP生产环境\<` + strings.Split(a.Labels["alertname"], `:`)[0] + `\>报警`
	return
}

func generateDingText(a *Alert) (text string) {
	item := strings.Split(a.Labels["alertname"], `:`)[0]
	text += `## ERP生产环境 \<` + item + `\> 报警` + "\n"
	text += `### 等级：` + generateLevelMsg(a.Labels["level"]) + "\n"
	text += `### 节点：` + a.Labels["node"] + "\n"
	text += `### 信息：` + a.Annotations["summary"] + "\n"
	text += `报警项: ` + a.Labels["alertname"] + "\n\n"
	text += fmt.Sprintf("[查看Prometheus指标详情](%s)\n\n", a.GeneratorURL)
	text += fmt.Sprintf("[查看Rabbitmq控制台](%s)\n\n", "http://"+viper.GetString("rabbit_host"))
	return
}

func generateRabbitQueuesOverview() (overview string) {
	return getTop10MostMsgsQueues() + getTop10PublishRateQueues() + getTop10AckRateQueues()
}

func getTop10MostMsgsQueues() (msg string) {
	msg += "------\n"
	msg += "消息堆积最多的队列top10\n"
	msg += "> | 队列名 | 消息数 |\n\n"
	for _, q := range TOP10_MOST_MSGS_QUEUES {
		msg += fmt.Sprintf("> | %s | %d | \n\n", q.Name, q.Messages)
	}
	msg += "\n"
	return
}

func getTop10PublishRateQueues() (msg string) {
	msg += "------\n"
	msg = "消息生产速率最快的队列top10\n"
	msg += "> | 队列名 | 消息生产速率 | 消息数 |\n\n"
	for _, q := range TOP10_PUBLISH_RATE_QUEUES {
		msg += fmt.Sprintf("> | %s | %.2f/s | %d |\n\n", q.Name, q.MessageStats.PublishDetails.Rate, q.Messages)
	}
	msg += "\n"
	return
}

func getTop10AckRateQueues() (msg string) {
	msg += "------\n"
	msg = "消息消费速率最快的队列top10\n"
	msg += "> | 队列名 | 消息消费速率 | 消息数 |\n\n"
	for _, q := range TOP10_ACK_RATE_QUEUES {
		msg += fmt.Sprintf("> | %s | %.2f/s | %d |\n\n", q.Name, q.MessageStats.AckDetails.Rate, q.Messages)
	}
	msg += "\n"
	return
}

func sendDingAlertMsg(n *Notification) {
	ding := client.DingTalk{
		AccessToken: viper.GetString("dingtalk_robot_token"),
		Secret:      viper.GetString("dingtalk_robot_secret"),
	}
	for _, a := range n.Alerts {
		msg := message.NewMarkdownMessage().SetMarkdown(
			generateDingTitle(&a), generateDingText(&a)+generateRabbitQueuesOverview(),
		).SetAt(strings.Split(a.Annotations["dingAt"], ","), false)
		dingSeverResp, err := ding.Send(msg)
		if err != nil {
			log.Errorf("send Ding msg failed: %s\nerr: %v\n", dingSeverResp.ErrMsg, err)
		}
		log.Infof("send Ding alart msg success, alert name: %s\n", a.Labels["alertname"])
	}
}

//func sendDingCtlRabbitMsg(action string){
//	ding := client.DingTalk{
//		AccessToken: DINGTAIK_ROBOT_TOKEN,
//		Secret: DINGTAIK_ROBOT_SECRET,
//	}
//}
//

func sendDingAutoRestartRabbitMsg(nodeIp string) {
	ding := client.DingTalk{
		AccessToken: viper.GetString("dingtalk_robot_token"),
		Secret:      viper.GetString("dingtalk_robot_secret"),
	}
	msg := message.NewMarkdownMessage().SetMarkdown(
		"ERP生产环境<rabbitmq>自动重启触发",
		fmt.Sprintf("## ⚡ERP生产环境\\<rabbitmq\\>\n## 节点[%s]\n## rabbitmq-server故障自动重启触发！\n", nodeIp),
	)
	dingSeverResp, err := ding.Send(msg)
	if err != nil {
		log.Errorf("send Ding msg failed: %s\nerr: %v\n", dingSeverResp.ErrMsg, err)
	}
	log.Infoln("send Ding auto-ctl msg success")
}
