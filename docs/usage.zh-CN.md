# 详细使用指南

[한국어](usage.md) | [English](usage.en.md) | [简体中文](usage.zh-CN.md) | [日本語](usage.ja.md)

subscale 是一个命令行工具，可在保留现有 Tailscale 连接的同时，通过多个已命名的辅助 Tailscale 账号访问指定地址。各账号的登录状态单独保存，无需切换现有的 Tailscale 账号。

**支持范围：** macOS/Linux 上的 HTTP/HTTPS 代理和 TCP 隧道。浏览器可以通过自动代理配置（PAC）仅为已注册的地址选择辅助连接。不支持对所有应用透明生效的 VPN/TUN 路由，也不支持 UDP/ICMP。

## 安装与自动启动

安装命令请参阅 [README](../README.zh-CN.md#安装)。如需手动下载，请在[最新版本下载页面](https://github.com/hyuck0221/subscale/releases/latest)选择适合你的操作系统和 CPU 的文件。

默认安装目录为 `~/.local/bin`。如果安装后找不到 `subscale` 命令，请在 shell 配置文件中添加以下内容：

```sh
export PATH="$HOME/.local/bin:$PATH"
```

也可以指定版本或安装目录。

```sh
curl -fsSL https://raw.githubusercontent.com/hyuck0221/subscale/main/scripts/install.sh | SUBSCALE_VERSION=v0.1.0 SUBSCALE_INSTALL_DIR="$HOME/.local/bin" sh
```

在 macOS 上，安装脚本会自动执行 `subscale service install`。对于已有的安装，执行一次 `subscale service install` 即可。如需跳过自动启动注册，请在运行安装脚本的 shell 中设置 `SUBSCALE_AUTOSTART=0`。注册失败时，程序文件仍会保留，并输出重试命令。在 Linux 上或手动下载程序文件后，请自行执行 `subscale service install`。

`service install` 会在你登录电脑时启动 subscale，并重新连接已保存的账号。如果移动了已安装的程序文件，需要重新安装服务。不支持在用户登录前的系统启动阶段运行。即使未安装服务，执行命令时也会自动启动守护进程。

## 注册和使用账号

```sh
subscale work login
subscale home login

subscale work add grafana.example.ts.net
subscale work add 100.80.1.2
subscale home add nas.other-tailnet.ts.net
subscale work add '*.corp.example'

subscale list                        # 仅显示已认证的账号
subscale status                      # 包括已退出登录的账号
subscale work list
subscale work remove grafana.example.ts.net  # 也可以使用 rm
subscale work logout
```

账号名称由 1～32 个英文字母、数字、`_` 或 `-` 组成，首字符必须为英文字母或数字。`status`、`list`、`update`、`upgrade`、`rename` 等命令及子命令名称为保留名称，不区分大小写。执行 `login` 时，请在浏览器中登录**要与该名称关联的 Tailscale 账号**。认证 URL 也会显示在终端中。如果不希望在服务器上打开浏览器，请设置 `SUBSCALE_NO_BROWSER=1`。

登录状态会保存，后续运行时无需再次登录即可连接。但 Tailscale 密钥过期、需要管理员批准、设备被删除或认证策略变更时，可能仍需重新认证。每个账号会注册为一个独立设备，例如 `subscale-work`，并受 tailnet 的设备数量限制和 ACL 规则约束。`logout` 仅退出该账号，保留名称和地址规则。之后可以通过 `login` 再次使用。

修改账号名称不会丢失登录状态或地址规则。再次使用旧名称登录时，会注册为一个独立的新账号。

```sh
subscale work rename office
subscale office list
```

不能改为已有的账号名称或命令名称。如果保存失败，将保留原名称和连接。

## 更新

```sh
subscale update
# 等效命令：subscale upgrade
```

subscale 会下载适合当前操作系统和 CPU 的最新版本，验证文件后安装。如果已经是最新版本，则不会替换文件。下载或验证失败时，将保留现有文件。你需要对安装目录具有写入权限。

正在运行的守护进程会重启，并保留已保存的账号和规则。重启过程中，现有代理和 SSH 连接可能会中断。

## 在浏览器中直接访问地址

在 macOS 上，指定网络服务名称，设置一次即可。

```sh
subscale proxy on Wi-Fi
# 使用有线网络时，请指定实际的网络服务名称。
# networksetup -listallnetworkservices
```

之后，在遵循系统自动代理设置的浏览器中，可以直接访问已注册的地址。原有的 PAC 设置会保存，可通过以下命令恢复。修改网络设置可能需要相应权限。

```sh
subscale proxy off Wi-Fi
```

在其他操作系统或使用独立代理设置的浏览器中，请将以下命令输出的地址填写为**自动代理配置 URL**：

```sh
subscale proxy pac
# http://127.0.0.1:18765/proxy.pac
```

已注册的地址通过指定的辅助账号连接，其他地址使用现有连接。浏览器可能缓存自动代理设置，因此修改规则后可能需要刷新页面或重启浏览器。辅助账号出错时，代理不会通过默认连接重试。但它无法控制浏览器在 PAC 下载失败时的处理方式、本地地址的代理绕过行为，或现有连接的复用。如需恢复，请关闭代理并重启守护进程。

## 终端程序与 SSH

```sh
subscale exec curl https://grafana.example.ts.net
subscale exec curl http://100.80.1.2:3000

ssh -o 'ProxyCommand=subscale connect %h %p' user@nas.other-tailnet.ts.net
```

通过 `exec` 运行的程序必须支持 HTTP/HTTPS 代理环境变量。启动任意应用或 UDP 程序，并不会使其流量自动通过 subscale。

常用的 SSH 连接可以写入 `~/.ssh/config`：

```sshconfig
Host nas.other-tailnet.ts.net
    ProxyCommand subscale connect %h %p
```

将 `connect` 设为 SSH 的 `ProxyCommand` 后，已注册地址使用对应的辅助账号，其他地址使用现有连接。

## 本地 TCP 端口转发

可用于屏幕共享、SSH、数据库等 TCP 服务。转发通过所选账号的 Tailscale 连接中继 TCP 流量，因此转发本身不需要 SSH 服务器、用户名或 SSH 认证。目标服务自身的认证要求仍然有效。

```sh
subscale mds forward add 100.89.207.112:5900
subscale mds forward add 100.89.207.112:5432 --local 15432
subscale mds forward add 'db.example.ts.net:5432'
subscale mds forward add '[fd7a:115c:a1e0::1]:22'
subscale mds forward list
subscale mds forward remove 100.89.207.112:5900
```

将客户端连接到 `add` 输出的 `127.0.0.1:端口`。屏幕共享可在 Finder 的“连接服务器”中使用 `vnc://127.0.0.1:端口`；数据库则使用主机 `127.0.0.1` 和对应端口。转发端口只能通过本机的 `127.0.0.1` 访问。无需配置浏览器 PAC，也无需另行添加主机路由规则。目标域名通过所选账号的网络解析。现有的 `add HOST --to HOST` 别名规则不会应用于转发目标。

省略 `--local` 时，会自动分配并保存一个可用端口。再次注册相同账号和目标时，会保留现有端口。如需更改本地端口，请先删除该转发再重新注册。同一个目标可以在不同账号下注册，并分别使用不同的本地端口。端口 `18765` 保留给代理使用。

守护进程运行期间，注册会立即生效；重启后会恢复相同端口。如需在电脑重启后自动恢复，请注册登录时自动启动的服务。如果端口已被占用，subscale 不会擅自更换端口，而会在 `forward list` 和 `status` 中显示错误。解决冲突后，可使用相同的 `forward add` 命令重试。即使显示 `listening`，远程设备或服务未运行时仍无法连接。

退出登录后会拒绝新连接，但保留转发条目。修改账号名称时，转发条目也会迁移到新名称。`remove` 会删除转发条目并终止正在进行的连接。不支持 UDP。

## 地址规则

- 支持域名、单标签 MagicDNS 名称、IPv4、IPv6 和 `*.example.com`。不支持 CIDR。
- `*.example.com` 仅匹配子域名。如果需要访问 `example.com` 本身，请单独注册。
- 优先使用精确匹配规则，其次使用最具体的通配符规则。
- 将同一主机重复注册到另一个账号会返回错误。请先删除现有规则。
- 即使输入 URL 或 `host:port`，规则也会应用于**该主机的所有 TCP 端口**。URL 的路径、协议和端口不会保存在规则中。
- 域名通过所选账号的网络解析。如果不同 tailnet 中存在相同的 `100.x` 地址，可使用别名和 `--to` 加以区分。

```sh
subscale work add work-db.internal --to 100.80.1.2
subscale home add home-db.internal --to 100.80.1.2
subscale exec curl http://work-db.internal:3000
```

`--to` 只修改目标地址，端口保持不变。通过 HTTPS 连接时，服务器证书必须与连接所用的别名匹配。仅注册域名不会使公网地址通过辅助 tailnet 作为互联网出口访问。此版本不提供出口节点（exit node）选择功能。访问子网地址时，辅助 tailnet 中必须存在通往该地址的路由。

## 保存的数据与故障排查

登录状态、地址规则和转发条目保存在以下位置。可以使用 `SUBSCALE_HOME` 更改保存目录。

- macOS：`~/Library/Application Support/subscale`
- Linux：`$XDG_CONFIG_HOME/subscale` 或 `~/.config/subscale`

该目录包含账号凭据，请勿与他人共享。遇到问题时，可以查看目录中的 `daemon.log`。日志也可能包含登录说明，共享前请先检查内容。

```sh
subscale status
subscale start
subscale stop
```

如果其他程序正在使用代理端口 `18765`，subscale 将无法启动。请检查冲突程序的端口设置。

## 卸载

如果在 macOS 上启用了自动代理，请先恢复原有设置，再取消自动启动服务并停止 subscale。将 `Wi-Fi` 替换为设置时使用的网络服务名称。如果未配置自动代理，可跳过第一条命令。

```sh
subscale proxy off Wi-Fi
subscale service uninstall
subscale stop
```

如果取消服务时守护进程已经停止，`stop` 可能提示没有正在运行的守护进程。然后删除安装目录中的 `subscale` 文件。默认路径为 `~/.local/bin/subscale`。

登录状态和地址规则不会自动删除。如需完全卸载，请同时删除上述数据目录。
