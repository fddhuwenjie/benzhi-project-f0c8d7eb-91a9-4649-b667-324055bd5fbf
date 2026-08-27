package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

const defaultAddress = "127.0.0.1:19081"

func resolveAddress(flagValue string) (string, error) {
	value := strings.TrimSpace(flagValue)
	if value == "" {
		if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
			if _, err := validPort(port); err != nil {
				return "", fmt.Errorf("PORT 不合法: %w", err)
			}
			return net.JoinHostPort("127.0.0.1", port), nil
		}
		return defaultAddress, nil
	}
	if _, err := strconv.Atoi(value); err == nil {
		if _, err := validPort(value); err != nil {
			return "", err
		}
		return net.JoinHostPort("127.0.0.1", value), nil
	}
	host, port, err := net.SplitHostPort(value)
	if err != nil {
		return "", fmt.Errorf("监听地址必须是 host:port: %w", err)
	}
	if strings.TrimSpace(host) == "" {
		return "", fmt.Errorf("监听地址不能省略主机")
	}
	if _, err := validPort(port); err != nil {
		return "", err
	}
	return net.JoinHostPort(host, port), nil
}

func validPort(value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("端口必须位于 1 到 65535")
	}
	return port, nil
}
