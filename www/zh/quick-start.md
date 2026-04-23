# 快速开始

本指南帮助你在约 5 分钟内跑通 ByteMind。

## 1. 安装

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash
```

## 2. 创建配置

```bash
mkdir -p .bytemind
cp config.example.json .bytemind/config.json
```

在 `.bytemind/config.json` 中填写 provider 和 API Key。

## 3. 启动聊天模式

```bash
bytemind chat
```

## 4. 执行第一个任务

示例提示词：

```text
定位失败测试并以最小改动修复。
```

## 下一步

- [安装](/zh/installation)
- [配置](/zh/configuration)
- [聊天模式](/zh/usage/chat-mode)
