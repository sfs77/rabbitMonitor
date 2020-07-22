package main

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"net/http"
	"os"
	"time"
)

func setupRouter() *gin.Engine {
	r := gin.Default()
	//Prometheus metrics端口
	r.GET("/rabbit/metrics", gin.WrapH(promhttp.HandlerFor(PROMETEUS_REGISTRY, promhttp.HandlerOpts{})))

	//获取rabbit status信息
	r.GET("/rabbit/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, LATEST_RABBIT_STATUS)
	})

	//获取rabbit队列的overview信息(top10的最多消息堆积、最大消息生产速率、最大消息消费速率的队列)
	r.GET("/rabbit/queues/overview", func(c *gin.Context) {
		c.Writer.Write([]byte(generateRabbitQueuesOverview()))
	})

	//启动、停止、重启rabbitmq
	r.POST("/rabbit/control/:nodeIp", func(c *gin.Context) {
		nodeIp := c.Param("nodeIp")
		var actionBody map[string]interface{}
		c.BindJSON(&actionBody)
		cliOutput, err := ctlRabbit(nodeIp, actionBody["action"].(string))
		httpCode := http.StatusOK
		var errStr string
		if err != nil {
			httpCode = http.StatusBadRequest
			errStr = err.Error()
		}
		c.JSON(httpCode, struct {
			CliOutput string `json:"cli_output"`
			Err       string `json:"err"`
		}{
			CliOutput: cliOutput,
			Err:       errStr,
		})
	})

	//Alertmanager webhook端口
	r.POST("/rabbit/alert-hook", alertHookHandler)

	//获取“允许故障自动重启mq”的标志位
	r.GET("/rabbit/auto-restart", func(c *gin.Context) {
		if viper.GetBool("auto_restart") {
			c.Status(http.StatusOK)
			c.Writer.Write([]byte(`auto-restart when rabbitmq nodes failed is enabled`))
		} else {
			c.Status(http.StatusBadRequest)
			c.Writer.Write([]byte(`auto-restart when rabbitmq nodes failed is disabled`))
		}
	})

	//将“允许故障自动重启mq”的标志位置为true
	r.PUT("/rabbit/auto-restart", func(c *gin.Context) {
		viper.Set("auto_restart", true)
		c.Writer.Write([]byte(`now set auto-restart when rabbitmq nodes failed to enabled`))
	})

	//将“允许故障自动重启mq”的标志位置为false
	r.DELETE("/rabbit/auto-restart", func(c *gin.Context) {
		viper.Set("auto_restart", false)
		c.Writer.Write([]byte(`now set auto-restart of rabbitmq nodes failed to disabled`))
	})

	r.GET("/", func(c *gin.Context) {
		_, _ = c.Writer.Write([]byte(`<html>
             <head><title>VO Rabbit Exporter</title></head>
             <body>
             <h1>VO Rabbit Exporter</h1>
             <p><a href='/rabbit/metrics'>Metrics</a></p>
             </body>
             </html>`))
	})

	return r
}

func main() {
	initConfig()
	initLogger()
	exporter := NewVoRabbitExporter()
	PROMETEUS_REGISTRY.MustRegister(exporter)
	monitorRouter := setupRouter()
	monitorServer := &http.Server{
		Addr:           ":" + HTTP_SERVER_PORT,
		Handler:        monitorRouter,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   70 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	//删除上次运行时建立的测试队列
	_ = deleteQueue(viper.GetString("rabbit_host"), viper.GetString("monitor_queue_name"))
	//10s执行一次的后台轮询
	ticker10s := time.NewTicker(10 * time.Second)
	go func() {
		for {
			<-ticker10s.C
			_ = getRabbitStatus(viper.GetString("rabbit_host"))
		}
	}()
	//30s执行一次的后台轮询
	ticker30s := time.NewTicker(30 * time.Second)
	go func() {
		for {
			<-ticker30s.C
			getQueuesOverview(viper.GetString("rabbit_host"))
		}
	}()
	//1m执行一次的测试队列消息补充
	ticker1m := time.NewTicker(1 * time.Minute)
	go func() {
		for {
			<-ticker1m.C
			_ = sendMsgOK(viper.GetString("rabbit_host"))
		}
	}()
	//启动http server
	if err := monitorServer.ListenAndServe(); err != nil {
		log.Errorf("Error occur when start http server %v", err)
		os.Exit(1)
	}
}
