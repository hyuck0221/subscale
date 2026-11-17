# subscale

[한국어](README.md) | [English](README.en.md) | [简体中文](README.zh-CN.md) | [日本語](README.ja.md)

保留现有的 Tailscale 连接，同时通过另一个 Tailscale 账号访问指定地址。

## 安装

### Homebrew

```sh
brew install hyuck0221/tap/subscale
```

### 安装脚本

```sh
curl -fsSL https://raw.githubusercontent.com/hyuck0221/subscale/main/scripts/install.sh | sh
```

## 使用方法

`[NAME]` 是你为账号设置的名称。输入命令时，请将占位符连同方括号一起替换为实际值。

```sh
subscale [NAME] login
subscale [NAME] add [HOST]
subscale [NAME] add '*.example.com' --to [HOST]
subscale [NAME] list
```

| 命令 | 说明 |
| --- | --- |
| `subscale list` | 列出已登录的账号 |
| `subscale status` | 查看所有账号的状态 |
| `subscale [NAME] devices` | 列出账号可见的设备 |
| `subscale [NAME] check [HOST]:[PORT]` | 诊断 DNS 和 TCP 连接 |
| `subscale [NAME] rename [NEW_NAME]` | 修改账号名称 |
| `subscale [NAME] remove [HOST]` | 删除地址规则 |
| `subscale [NAME] logout` | 退出登录 |
| `subscale update` | 更新到最新版本 |

## 浏览器连接

请将 `Wi-Fi` 替换为正在使用的 macOS 网络服务名称。

```sh
subscale proxy on Wi-Fi
subscale proxy off Wi-Fi
```

在其他环境中，请将 `subscale proxy pac` 输出的 URL 填入浏览器的自动代理设置。

## TCP 端口转发

```sh
subscale [NAME] forward add [HOST]:[PORT]
subscale [NAME] forward add [HOST]:[PORT] --local [LOCAL_PORT]
subscale [NAME] forward list
subscale [NAME] forward remove [HOST]:[PORT]
```

连接到输出的 `127.0.0.1:端口`。本地端口会自动分配并保存，在守护进程重启时恢复。

## 终端与 SSH

```sh
subscale exec curl https://[HOST]
ssh -o 'ProxyCommand=subscale connect %h %p' [USER]@[HOST]
```

[详细使用指南](docs/usage.zh-CN.md)
