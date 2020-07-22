package main

import (
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"net/http"
	"path/filepath"
	"sync"
	"time"
)

func initConfig() {
	//viper环境变量
	viper.SetEnvPrefix("vo")
	viper.BindEnv("auto_restart")          //是否允许rabbitmq节点故障时自动重启
	viper.BindEnv("dingtalk_robot_token")  //钉钉通知群机器人access_token
	viper.BindEnv("dingtalk_robot_secret") //钉钉通知群机器人secret
	viper.BindEnv("monitor_nodes_name")    //rabbitmq监控的节点hostname或IP, 逗号分隔
	viper.BindEnv("monitor_queue_name")    //rabbitmq监控测试用队列名称，默认为"vo_rabbit_monitor"
	viper.BindEnv("rabbit_cli")            //rabbitmq ssh 命令行控制命令前缀，默认为"sudo service rabbitmq-server"
	viper.BindEnv("rabbit_host")           //用于发送接受消息，发送api请求的rabbitmq集群hostname或者IP，一般应该为前置的负载均衡
	viper.BindEnv("rabbit_user")           //rabbitmq用户名
	viper.BindEnv("rabbit_password")       //rabbitmq用户密码
	viper.BindEnv("ssh_user")              //rabbitmq节点ssh用户，应在每个节点上都存在此用户，且有sudo权限
	viper.BindEnv("ssh_password")          //rabbit节点ssh用户密码
	//viper flag 命令行参数
	var configPath string
	pflag.StringVarP(&configPath, "config", "c", "", "指定配置文件位置")
	pflag.StringVarP(&HTTP_SERVER_PORT, "port", "p", "8080", "http服务器的监听端口")
	pflag.Parse()
	viper.BindPFlags(pflag.CommandLine)
	//viper 配置文件
	configDir := filepath.Dir(configPath)   //截取文件路径的dir
	configBase := filepath.Base(configPath) //截取文件路径的filename
	if configBase == "" || configBase == "." || configBase == "/" {
		viper.SetConfigName("vo_rabbit_monitor")
	} else {
		viper.SetConfigName(configBase)
	}
	viper.AddConfigPath("/etc/vo_rabbit_monitor")
	viper.AddConfigPath(".")
	viper.AddConfigPath(configDir)
	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		log.Errorf("error when read config file: %s \n", err)
	}
	//viper 设置默认值
	viper.SetDefault("auto_restart", true)
	viper.SetDefault("monitor_queue_name", "vo_rabbit_monitor")
	viper.SetDefault("rabbit_cli", "sudo service rabbitmq-server")
	//配置非空验证
	neededConfigKeys := []string{"dingtalk_robot_token", "monitor_nodes_name",
		"monitor_queue_name", "rabbit_host", "rabbit_user", "rabbit_password", "ssh_user", "ssh_password"}
	for _, key := range neededConfigKeys {
		if viper.GetString(key) == "" {
			log.Panicf("Fatal error: config key \"%s\" must not be empty", key)
		}
	}
}

var (
	HTTP_CLIENT               *http.Client = &http.Client{Timeout: 5 * time.Second}
	HTTP_SERVER_PORT          string
	LATEST_RABBIT_STATUS      []nodeStatus
	PROMETEUS_REGISTRY        = prometheus.NewRegistry()
	SSH_RWMUTEX               sync.RWMutex
	TOP10_MOST_MSGS_QUEUES    []queueStatus
	TOP10_PUBLISH_RATE_QUEUES []queueStatus
	TOP10_ACK_RATE_QUEUES     []queueStatus
)

const (
	DEFAULT_LOGLEVEL            = log.InfoLevel
	PROMETHEUS_NAMESPACE        = "vorabbit"
	DEFAULT_RABBIT_CLUSTER_NAME = "GatewayPlus"
)

type MyError string

func (e MyError) Error() string {
	return string(e)
}
