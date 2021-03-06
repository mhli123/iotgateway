//go:generate go-bindata -prefix ../../message/ -pkg message -o ../../internal/message/message_gen.go ../../message/
//go:generate go-bindata -prefix ../../templates/ -pkg templates -o ../../internal/templates/templates_gen.go ../../templates/
package main

import (
	"container/list"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	//	"strings"
	//"runtime"
	"syscall"
	"time"
	//	"io/ioutil"
	//	"net"
	//	"net/http"
	log "github.com/Sirupsen/logrus"
	assetfs "github.com/elazarl/go-bindata-assetfs"
	"github.com/urfave/cli"
	"github.com/yjiong/iotgateway/internal/common"
	"github.com/yjiong/iotgateway/internal/device"
	gw "github.com/yjiong/iotgateway/internal/gateway"
	"github.com/yjiong/iotgateway/internal/handler"
	"github.com/yjiong/iotgateway/internal/message"
	"github.com/yjiong/iotgateway/internal/templates"
	"golang.org/x/net/websocket"
	"google.golang.org/grpc/grpclog"
)

func init() {
	//	if runtime.GOOS == "linux" {
	//
	//	}
	grpclog.SetLogger(log.StandardLogger())
	_, err := os.Stat(common.DEVFILEPATH)
	if os.IsNotExist(err) {
		f, _ := os.Create(common.DEVFILEPATH)
		//		f.WriteString("[devlist]")
		f.Sync()
		f.Close()
	}
	fmt.Printf("%s\n%s\n%s\n%s\n%s\n%s\n", "      _           _ ", "__  _(_)_ __   __| | ___  _ __   __ _ ", `\ \/ / | '_ \ / _' |/ _ \| '_ \ / _' |`,
		" >  <| | | | | (_| | (_) | | | | (_| |", `/_/\_\_|_| |_|\__,_|\___/|_| |_|\__, |`, `                                |___/ `)
}

func run(c *cli.Context) error {
	log.SetLevel(log.Level(uint8(c.Int("log-level"))))

	ctx := context.Background()

	ctx, cancel := context.WithCancel(ctx)

	defer cancel()
	log.WithFields(log.Fields{
		"version": common.VERSION,
		"docs":    "https://github.com/yjiong/iotgateway",
	}).Info("starting iot gateway programer")
	// 初始化
	//	http.Handle("/js", http.FileServer(http.Dir("templates")))
	//	http.HandleFunc("/", yjhttp)
	gateway := mustGetGateway(c)
	go func() {
		//http.Handle("/", http.FileServer(http.Dir(common.TEMPLATE)))
		http.Handle("/", http.FileServer(&assetfs.AssetFS{
			Asset:     templates.Asset,
			AssetDir:  templates.AssetDir,
			AssetInfo: templates.AssetInfo,
			Prefix:    "",
		}))
		//http.Handle("/msg/", http.StripPrefix("/msg/", http.FileServer(http.Dir(common.MESSAGE))))
		http.Handle("/msg/", http.StripPrefix("/msg/", http.FileServer(&assetfs.AssetFS{
			Asset:     message.Asset,
			AssetDir:  message.AssetDir,
			AssetInfo: message.AssetInfo,
			Prefix:    "",
		})))
		http.Handle("/message", websocket.Handler(gateway.WsHandle))
		if err := http.ListenAndServe(":8000", nil); err != nil {
			log.Fatal("ListenAndServe:", err)
		}
	}()
	//websocket 消息处理
	go func() {
		for wscmd := range gateway.WsNochanr {
			for k, v := range wscmd {
				go gateway.Wscmdhandler(v, gateway.WsMap[k])
			}
		}
	}()

	go func() {
		/////////////////////////用chan方式实现命令处理///////////////////////////////
		for cmd := range gateway.Cmdchan {
			go cmd.Cmdfunc(cmd.Param)
		}
		//////////////////////////用队列的方式实现命令处理//////////////////////////////
		//		for{
		//				if gateway.Cmdlist.Len() != 0 {
		//				cmdfp := gateway.Cmdlist.Back()
		//				gateway.Cmdlist.Remove(cmdfp)
		//				cmdv, ok := cmdfp.Value.(devcmd.Cmdfp)
		//				if ok {
		//					cmdv.Cmdfunc(cmdv.Param)
		//				}
		//			}else{
		//				time.Sleep(time.Second)
		//			}
		//		}
		///////////////////////////////////////////////////////////////////////////////
	}()

	//按interval设置的时间间隔读取设备(单位秒),如果设置值为0,则不自动读取.
	go func() {
		for {
			pubInterval, err := strconv.Atoi(gateway.ConMap["_interval"])
			if err != nil {
				pubInterval = c.Int("interval")
			}
			time.Sleep(time.Second * time.Duration(pubInterval))
			//starttime := time.Now().Unix()
			if pubInterval != 0 {
				for id, dev := range gateway.DevIfMap {
					let, err := dev.RWDevValue("r", nil)
					if err != nil {
						let = map[string]interface{}{
							"_devid": id,
							"error":  err.Error(),
						}
					}
					if err := gateway.EncodeAutoup(let); err != nil {
						log.Errorf("auto updata error : %s", err)
					}
					///////////////////////测试时延时1秒,最终要取消??///////////////////////////////////
					//time.Sleep(time.Second)
					////////////////////////////////////////////////////////////////////////////////////
				}
			} else {
				time.Sleep(time.Second)
			}
			//log.Infoln("一个周期=", time.Now().Unix()-starttime)
			//break
		}
	}()

	mqttconnect(c, &gateway)
	//接受消息命令并执行
	go gateway.Mqttcmdhandler(gateway.Handler.DataDownChan())

	sigChan := make(chan os.Signal)
	exitChan := make(chan struct{})
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	log.WithField("signal", <-sigChan).Info("signal received")
	go func() {
		log.Warning("stopping mq")
		exitChan <- struct{}{}
	}()
	//	select {
	//	case a := <-exitChan:
	//		log.WithField("signal", a).Info("signal received, stopping immediately")
	//	case s := <-sigChan:
	//		log.WithField("signal", s).Info("signal received, stopping immediately")
	//	}

	return nil
}

func mustGetGateway(c *cli.Context) gw.Gateway {
	// 初始化设备,也就是设备的驱动接口
	devm, err := device.NewDevHandler(common.DEVFILEPATH)
	if err != nil {
		log.Fatalf("setup device interface map error: %s", err)
	}
	// 初始化配置文件
	conm, err := common.NewConMap(common.CONFILEPATH)
	if err != nil {
		log.Errorf("setup config parameter map error: %s", err)
	}

	return gw.Gateway{
		DevIfMap: devm,
		ConMap:   conm,
		//		Handler:   h,
		WsMap:     make(map[int]*websocket.Conn),
		Cmdlist:   list.New(),
		Devpath:   common.DEVFILEPATH,
		Conpath:   common.CONFILEPATH,
		Cmdchan:   make(chan gw.Cmdfp),
		WsNochanr: make(chan map[int]string),
	}
}

func mqttconnect(c *cli.Context, gateway *gw.Gateway) {
	// 初始化mqtt接口
	// 初始化配置文件
	conm, err := common.NewConMap(common.CONFILEPATH)
	var h handler.Handler
	willmsg := gateway.OnOfflineMsg(0)
	onlinemsg := gateway.OnOfflineMsg(1)
	cm := map[string]string{
		"_server_ip":   c.String("mqtt-server"),
		"_server_port": "",
		"_username":    c.String("mqtt-username"),
		"_password":    c.String("mqtt-password"),
		"cafile":       c.String("mqtt-ca-cert"),
		"_client_id":   c.String("client_id"),
		"_server_name": c.String("server_id"),
		"_keepalive":   "60",
	}
	if conm == nil {
		h, err = handler.NewMQTTHandler(cm,
			willmsg,
			onlinemsg)
		if err != nil {
			log.Fatalf("setup mqtt handler error: %s", err)
		}
	} else {
		h, err = handler.NewMQTTHandler(conm, willmsg, onlinemsg)
		if err != nil {
			log.Fatalf("setup mqtt handler error: %s", err)
		}
	}
	gateway.Handler = h
}

func main() {
	app := cli.NewApp()
	app.Name = "GATEWAY"
	app.Usage = "application for IOT gateway"
	app.Version = common.VERSION
	app.Author = "yaojiong"
	app.Email = "yjiong@msn.com"
	app.Copyright = "See https://github.com/yjiong/iotgateway for copyright information"
	app.Action = run
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "mqtt-server",
			Usage:  "mqtt server (e.g. scheme://host:port where scheme is tcp, ssl or ws)",
			Value:  "tcp://192.168.1.160:1883",
			EnvVar: "MQTT_SERVER",
		},
		cli.StringFlag{
			Name:   "mqtt-username",
			Usage:  "mqtt server username (optional)",
			Value:  "yj",
			EnvVar: "MQTT_USERNAME",
		},
		cli.StringFlag{
			Name:   "mqtt-password",
			Usage:  "mqtt server password (optional)",
			Value:  "yj12345",
			EnvVar: "MQTT_PASSWORD",
		},
		cli.StringFlag{
			Name:   "mqtt-ca-cert",
			Usage:  "mqtt CA certificate file used by the gateway backend (optional)",
			EnvVar: "MQTT_CA_CERT",
		},
		cli.IntFlag{
			Name:   "log-level",
			Value:  4,
			Usage:  "debug=5, info=4, warning=3, error=2, fatal=1, panic=0",
			EnvVar: "LOG_LEVEL",
		},
		cli.StringFlag{
			Name:   "client_id",
			Value:  "IotGW-GOLANG",
			Usage:  "subscribe publish topic client strings",
			EnvVar: "CLIENT_ID",
		},
		cli.StringFlag{
			Name:   "server_id",
			Value:  "iotserver",
			Usage:  "subscribe publish topic server strings",
			EnvVar: "SERVER_ID",
		},
		cli.IntFlag{
			Name:  "interval",
			Value: 300,
			Usage: "auto updata interval",
			EnvVar: "INTERVAL	",
		},
	}
	app.Run(os.Args)
}
