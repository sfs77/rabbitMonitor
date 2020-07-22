package main

import (
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"net/http"
	"strings"
	"time"
)

type Notification struct {
	Version           string            `json:"version"`
	GroupKey          string            `json:"groupKey"`
	Status            string            `json:"status"`
	Receiver          string            `json:"receiver"`
	GroupLabels       map[string]string `json:"groupLabels"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	ExternalURL       string            `json:"externalURL"`
	Alerts            []Alert           `json:"alerts"`
}

type Alert struct {
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
}

func alertHookHandler(c *gin.Context) {
	var notification Notification
	err := c.BindJSON(&notification)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	go sendDingAlertMsg(&notification)
	//如果告警等级有为FORCE_RESTART的等级，并且rabbitMoniter应用的”允许故障自动重启“标志位被置true, 则重启rabbit node, 并钉钉通知
	for _, a := range notification.Alerts {
		if a.Labels["level"] == "FORCE_RESTART" && viper.GetBool("auto_restart") {
			hostname := strings.Split(a.Labels["node"], "@")[1]
			go sendDingAutoRestartRabbitMsg(hostname)
			go ctlRabbit(hostname, "restart")
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "ding alert msg sending triggered"})
}
