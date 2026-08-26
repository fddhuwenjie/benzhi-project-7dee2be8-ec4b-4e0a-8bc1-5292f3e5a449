package main

import (
	"errors"
	"flag"
	"net"
	"os"
	"strings"
	"time"
)

type config struct {
	addr, dataDir string
	selfCheck     bool
	timeout       time.Duration
}

func parseConfig() (config, error) {
	var c config
	def := "127.0.0.1:19081"
	if p := os.Getenv("PORT"); p != "" {
		def = "127.0.0.1:" + p
	}
	flag.StringVar(&c.addr, "addr", def, "HTTP 监听地址")
	flag.StringVar(&c.dataDir, "data-dir", "./data", "本地持久化目录")
	flag.BoolVar(&c.selfCheck, "self-check", false, "执行真实 HTTP 闭环自检后退出")
	flag.DurationVar(&c.timeout, "self-check-timeout", 15*time.Second, "自检超时")
	flag.Parse()
	host, port, e := net.SplitHostPort(c.addr)
	if e != nil {
		return c, e
	}
	if port == "" {
		return c, errors.New("监听端口不能为空")
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return c, errors.New("监听地址必须为回环地址")
	}
	if strings.TrimSpace(c.dataDir) == "" {
		return c, errors.New("data-dir 不能为空")
	}
	return c, nil
}
