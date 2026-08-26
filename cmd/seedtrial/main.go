package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"seed-vigor-gate/internal/application"
	"seed-vigor-gate/internal/httpapi"
	"seed-vigor-gate/internal/store"
)

func main() {
	if err := run(); err != nil {
		log.Printf("seedtrial: %v", err)
		os.Exit(1)
	}
}
func run() error {
	c, e := parseConfig()
	if e != nil {
		return e
	}
	st, e := store.New(c.dataDir)
	if e != nil {
		return fmt.Errorf("打开数据目录: %w", e)
	}
	if e = st.Validate(); e != nil {
		return fmt.Errorf("恢复校验: %w", e)
	}
	app := application.New(st)
	srv := &http.Server{Addr: c.addr, Handler: httpapi.New(app), ReadHeaderTimeout: 5 * time.Second}
	ln, e := net.Listen("tcp", c.addr)
	if e != nil {
		return e
	}
	done := make(chan error, 1)
	go func() {
		e := srv.Serve(ln)
		if errors.Is(e, http.ErrServerClosed) {
			e = nil
		}
		done <- e
	}()
	if c.selfCheck {
		ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
		defer cancel()
		e = performSelfCheck(ctx, "http://"+ln.Addr().String())
		shutdown(srv)
		<-done
		return e
	}
	log.Printf("种子活力复核工作台已启动: http://%s", ln.Addr())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sig:
		shutdown(srv)
		return <-done
	case e = <-done:
		return e
	}
}
func shutdown(s *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.Shutdown(ctx)
}
