# BENZHI_README

## 项目说明
- 项目：benzhi-project-f0c8d7eb-91a9-4649-b667-324055bd5fbf
- 项目用途：面向射电天文台数据值守员与科学复核员的观测数据发布资格工作台，完整实现基线冻结、数据段登记、确定性质检、隔离补观、独立抽审、终态清单封存及摘要复算验证。
- Go 工具链：`golang:1.22`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 项目描述
- 项目名称：radio-observation-release-gate
- 项目介绍：面向射电天文台数据值守员与科学复核员的观测数据发布资格工作台，用一条可追溯流程完成观测基线冻结、数据段登记、射频干扰质检、隔离补观、独立抽审和不可变发布清单封存。
- 项目概述：面向射电天文台数据值守员与科学复核员的观测数据发布资格工作台，用一条可追溯流程完成观测基线冻结、数据段登记、射频干扰质检、隔离补观、独立抽审和不可变发布清单封存。
- 核心工作流：观测批次由草稿建档进入基线冻结，值守员登记全部数据段及证据摘要并执行确定性干扰质检；不合格数据段转入隔离并通过补观替换后重新质检，全部通过后生成确定性抽样清单，由独立复核员逐项裁定，最终将批次批准为不可变发布清单或拒绝封存。
- 对外接口：Go 服务直接提供无需 Node 构建的原生单页浏览器工作台及同源 JSON 端点；页面以批次状态、数据段表格、质检问题队列、抽审面板和发布清单校验视图承载完整主流程。服务通过 -addr 配置监听地址，也读取 PORT 并绑定到 127.0.0.1:<PORT>，默认监听 127.0.0.1:19081，绝不默认绑定常见低位端口或 0.0.0.0。

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...

cd /app && GOTOOLCHAIN=local go run ./cmd/server -selftest -addr=127.0.0.1:19091

cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh

./build_benzhi_docker.sh benzhi-project-f0c8d7eb-91a9-4649-b667-324055bd5fbf-amd64 linux/amd64

./build_benzhi_docker.sh benzhi-project-f0c8d7eb-91a9-4649-b667-324055bd5fbf-arm64 linux/arm64

docker run -it benzhi-project-f0c8d7eb-91a9-4649-b667-324055bd5fbf-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/server -selftest -addr=127.0.0.1:19091`
