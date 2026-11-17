# subscale

[한국어](README.md) | [English](README.en.md) | [简体中文](README.zh-CN.md) | [日本語](README.ja.md)

Connect to selected addresses through another Tailscale account while keeping your existing Tailscale connection.

## Installation

### Homebrew

```sh
brew install hyuck0221/tap/subscale
```

### Installation script

```sh
curl -fsSL https://raw.githubusercontent.com/hyuck0221/subscale/main/scripts/install.sh | sh
```

## Usage

`[NAME]` is an account name you choose. Replace each placeholder, including its square brackets, with an actual value.

```sh
subscale [NAME] login
subscale [NAME] add [HOST]
subscale [NAME] add '*.example.com' --to [HOST]
subscale [NAME] list
```

| Command | Description |
| --- | --- |
| `subscale list` | List signed-in accounts |
| `subscale status` | Show the status of all accounts |
| `subscale [NAME] rename [NEW_NAME]` | Rename an account |
| `subscale [NAME] remove [HOST]` | Remove an address rule |
| `subscale [NAME] logout` | Sign out |
| `subscale update` | Update to the latest version |

## Browser connections

Replace `Wi-Fi` with the name of your macOS network service.

```sh
subscale proxy on Wi-Fi
subscale proxy off Wi-Fi
```

For other environments, enter the URL printed by `subscale proxy pac` in your browser's automatic proxy settings.

## TCP port forwarding

```sh
subscale [NAME] forward add [HOST]:[PORT]
subscale [NAME] forward add [HOST]:[PORT] --local [LOCAL_PORT]
subscale [NAME] forward list
subscale [NAME] forward remove [HOST]:[PORT]
```

Connect to the displayed `127.0.0.1:port`. Local ports are assigned automatically, saved, and restored when the daemon restarts.

## Terminal and SSH

```sh
subscale exec curl https://[HOST]
ssh -o 'ProxyCommand=subscale connect %h %p' [USER]@[HOST]
```

[Detailed usage guide](docs/usage.en.md)
