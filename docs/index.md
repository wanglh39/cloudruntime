# Cloud Runtime — Mini 教学项目

> 从零理解云运行时的底层原理与设计哲学

## 项目简介

本项目是一个**教学性质的 Mini Cloud Runtime 实现**，旨在帮助开发者从底层原理理解云运行时（如 AWS Lambda、Google Cloud Functions 等）是如何工作的。

## 文档

完整的教程文档请访问：[文档站](https://wanglh39.github.io/cloudruntime/)

### 文档目录

| 模块 | 主题 | 核心问题 |
|------|------|---------|
| 模块 0 | [全局概览](docs/00-overview/) | Cloud Runtime 是什么？为什么需要它？ |
| 模块 1 | [隔离与沙箱](docs/01-isolation/) | 如何在同一台机器上安全运行互不信任的程序？ |
| 模块 2 | [调度](docs/02-scheduling/) | 如何决定哪个任务在哪个资源上运行？ |
| 模块 3 | [网络](docs/03-networking/) | 沙箱如何与外界通信？ |
| 模块 4 | [存储](docs/04-storage/) | 沙箱如何持久化数据？ |
| 模块 5 | [FaaS](docs/05-faas/) | 如何实现函数即服务？ |
| 模块 6 | [控制面](docs/06-control-plane/) | 如何管理整个系统的生命周期？ |

## 快速开始

```bash
# 构建并运行
go build -o cloudruntime .
./cloudruntime

# 运行测试
go test ./...
```

## 技术栈

- **语言**: Go 1.22
- **文档**: MkDocs Material
- **部署**: GitHub Pages