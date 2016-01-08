package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"encoding/json"
	"strings"

	yagnats "github.com/cloudfoundry/yagnats"

	"github.com/kelseyhightower/confd/log"
	template "github.com/kelseyhightower/confd/resource/template"
)

type Tag struct {
	Component string
}

type Url struct {
	Uris []string `json:"uris"`
	Tags Tag      `json:"tags"`
}

func main() {
	flag.Parse()
	if printVersion {
		fmt.Printf("confd %s\n", Version)
		os.Exit(0)
	}
	if err := initConfig(); err != nil {
		log.Fatal(err.Error())
	}

	log.Info("Starting confd")

	if onetime {
		if err := template.Process(templateConfig); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	stopChan := make(chan bool)
	doneChan := make(chan bool)
	errChan := make(chan error, 10)

	var processor template.Processor
	processor = template.IntervalProcessor(templateConfig, stopChan, doneChan, errChan, config.Interval)

	internal_url := []string{"uaa.paas.cn", "login.paas.cn", "loggregator.paas.cn", "api.paas.cn", "doppler.paas.cn", "hm9000.paas.cn"}
	internal := map[string]int{}
	for _, v := range internal_url {
		internal[v] = 1
	}

	//log.Printf("Receiving messages...\n")
	client := yagnats.NewClient()

	err := client.Connect(&yagnats.ConnectionInfo{
		Addr:     "10.27.227.205:4222",
		Username: "nats",
		Password: "nats",
	})
	if err != nil {
		panic("Wrong auth or something.")
	}

	client.Subscribe("router.register", func(msg *yagnats.Message) { //订阅域名创建消息
		//fmt.Println(string(msg.Payload))

		url := Url{}
		json.Unmarshal(msg.Payload, &url)
		if !strings.Contains(url.Tags.Component, "directory-server-") {
			for _, v := range url.Uris {
				_, err := internal[v]
				if !err {
					flag := 0
					for _, value := range template.AllUrl {
						if value == v {
							flag = 1
							break
						}
					}
					if flag == 1 {
						continue
					}
					filename := "/var/vcap/nfs/shared/pems/" + v + ".pem"
					fmt.Println(filename)
					_, exist := os.Stat(filename)
					if exist == nil {
						template.AllUrl = append(template.AllUrl, v)
					}
				}
			}
			processor.Process()
		}
	})

	client.Subscribe("router.unregister", func(msg *yagnats.Message) { //订阅域名删除消息
		fmt.Println(string(msg.Payload))

		url := Url{}
		json.Unmarshal(msg.Payload, &url)
		if !strings.Contains(url.Tags.Component, "directory-server-") {
			for _, v := range url.Uris {
				_, err := internal[v]
				if !err {
					for k, a := range template.AllUrl {
						if v == a {
							kk := k + 1
							template.AllUrl = append(template.AllUrl[:k], template.AllUrl[kk:]...)
							break
						}
					}
				}
			}
			processor.Process()
		}
	})

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	for {
		select {
		case err := <-errChan:
			log.Error(err.Error())
		case s := <-signalChan:
			log.Info(fmt.Sprintf("Captured %v. Exiting...", s))
			close(doneChan)
		case <-doneChan:
			os.Exit(0)
		}
	}

}
