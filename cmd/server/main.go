package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"radio-observation-release-gate/internal/application"
	"radio-observation-release-gate/internal/storage"
	"radio-observation-release-gate/internal/web"
)

func main() {
	var address string
	var dataDir string
	var selftest bool
	flag.StringVar(&address, "addr", "", "监听地址，纯端口或 host:port")
	flag.StringVar(&dataDir, "data-dir", "data", "持久化数据目录")
	flag.BoolVar(&selftest, "selftest", false, "通过真实 HTTP 执行完整自检后退出")
	flag.Parse()
	resolved, err := resolveAddress(address)
	if err != nil {
		fatal(err)
	}
	if selftest {
		if err := runSelftest(resolved); err != nil {
			fatal(err)
		}
		fmt.Println("自检通过：观测批次已批准，发布清单摘要有效")
		return
	}
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		fatal(err)
	}
	repo, err := storage.New(abs)
	if err != nil {
		fatal(err)
	}
	if _, err := repo.VerifyAll(); err != nil {
		fatal(fmt.Errorf("启动恢复校验失败: %w", err))
	}
	app := application.New(repo)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	server := &http.Server{Addr: resolved, Handler: web.New(app, logger), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	listener, err := net.Listen("tcp", resolved)
	if err != nil {
		fatal(err)
	}
	logger.Info("观测发布资格工作台已启动", "addr", listener.Addr().String(), "data_dir", abs)
	errCh := make(chan error, 1)
	go func() { errCh <- server.Serve(listener) }()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-signals:
		logger.Info("收到关闭信号", "signal", sig.String())
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		fatal(err)
	}
}

func fatal(err error) { fmt.Fprintln(os.Stderr, "错误:", err); os.Exit(1) }
