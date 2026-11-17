# Detailed usage guide

[한국어](usage.md) | [English](usage.en.md) | [简体中文](usage.zh-CN.md) | [日本語](usage.ja.md)

subscale is a CLI for accessing selected addresses through multiple named secondary Tailscale accounts while keeping your existing Tailscale connection. Sign-in state is saved separately for each account, so you do not need to switch your existing Tailscale account.

**Supported connections:** HTTP/HTTPS proxies and TCP tunnels on macOS and Linux. Browsers can use automatic proxy configuration (PAC) to route only registered addresses. Transparent VPN/TUN routing for all applications and UDP/ICMP are not supported.

## Installation and automatic startup

See the [README](../README.en.md#installation) for the installation command. To download a file manually, choose the archive for your OS and CPU from the [latest downloads](https://github.com/hyuck0221/subscale/releases/latest).

The default installation directory is `~/.local/bin`. If the `subscale` command is not found after installation, add this line to your shell configuration file:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

You can also choose a specific version or installation directory.

```sh
curl -fsSL https://raw.githubusercontent.com/hyuck0221/subscale/main/scripts/install.sh | SUBSCALE_VERSION=v0.1.0 SUBSCALE_INSTALL_DIR="$HOME/.local/bin" sh
```

On macOS, the installer automatically runs `subscale service install`. For an existing installation, run `subscale service install` once. To skip automatic startup registration, set `SUBSCALE_AUTOSTART=0` in the shell running the installer. If registration fails, the binary remains installed and a retry command is printed. On Linux, or if you download the binary manually, run `subscale service install` yourself.

`service install` starts subscale when you log in to your computer and reconnects your saved accounts. If you move the installed binary, reinstall the service. Running before user login during system boot is not supported. Without an installed service, running a command still starts the daemon automatically.

## Registering and using accounts

```sh
subscale work login
subscale home login

subscale work add grafana.example.ts.net
subscale work add 100.80.1.2
subscale home add nas.other-tailnet.ts.net
subscale work add '*.corp.example'

subscale list                        # Authenticated accounts only
subscale status                      # Includes signed-out accounts
subscale work list
subscale work remove grafana.example.ts.net  # rm also works
subscale work logout
```

Account names must contain 1–32 ASCII letters, digits, `_`, or `-`, starting with a letter or digit. Command and subcommand names such as `status`, `list`, `update`, `upgrade`, and `rename` are reserved, regardless of case. During `login`, sign in through your browser with **the Tailscale account you want to associate with that name**. The authentication URL is also printed in the terminal. To avoid opening a browser on a server, set `SUBSCALE_NO_BROWSER=1`.

Sign-in state is saved, so subsequent runs reconnect without another login. Reauthentication may still be required if a Tailscale key expires, administrator approval is needed, a device is removed, or authentication policies change. Each account is registered as a separate device, such as `subscale-work`, and is subject to the tailnet's device limits and ACLs. `logout` signs out only that account, keeping its name and address rules. Use `login` to connect it again.

You can rename an account without losing its sign-in state or address rules. Signing in again under the old name registers a separate new account.

```sh
subscale work rename office
subscale office list
```

You cannot rename an account to an existing account name or a command name. If saving fails, the original name and connection are retained.

## Updating

```sh
subscale update
# Equivalent command: subscale upgrade
```

subscale downloads the latest version for your OS and CPU, verifies the file, and installs it. If you already have the latest version, nothing is replaced. If downloading or verification fails, the existing file is kept. You need write permission for the installation directory.

A running daemon restarts while retaining saved accounts and rules. Existing proxy and SSH connections may be disconnected during the restart.

## Opening registered addresses in a browser

On macOS, configure the proxy once using your network service name.

```sh
subscale proxy on Wi-Fi
# For a wired network, specify its actual service name.
# networksetup -listallnetworkservices
```

You can then open registered addresses directly in browsers that follow the system's automatic proxy settings. Previous PAC settings are saved and can be restored with the following command. Permission to change network settings may be required.

```sh
subscale proxy off Wi-Fi
```

On other operating systems, or in browsers with separate proxy settings, use the address printed by this command as the **automatic proxy configuration URL**:

```sh
subscale proxy pac
# http://127.0.0.1:18765/proxy.pac
```

Registered addresses use the assigned secondary account; other addresses use your existing connection. Browsers may cache automatic proxy settings, so changing rules may require a refresh or browser restart. If a secondary account fails, the proxy does not retry over the default connection. It cannot control how a browser handles PAC download failures, bypasses the proxy for local addresses, or reuses existing connections. To recover, turn off the proxy and restart the daemon.

## Terminal programs and SSH

```sh
subscale exec curl https://grafana.example.ts.net
subscale exec curl http://100.80.1.2:3000

ssh -o 'ProxyCommand=subscale connect %h %p' user@nas.other-tailnet.ts.net
```

Programs launched with `exec` must support HTTP/HTTPS proxy environment variables. Launching an arbitrary application or UDP program does not automatically route its traffic through subscale.

For frequently used SSH connections, add an entry to `~/.ssh/config`:

```sshconfig
Host nas.other-tailnet.ts.net
    ProxyCommand subscale connect %h %p
```

With `connect` as SSH's `ProxyCommand`, registered addresses use the corresponding secondary account and other addresses use your existing connection.

## Local TCP port forwarding

Use port forwarding for TCP services such as screen sharing, SSH, or databases. Connections are relayed through the selected account's Tailscale connection, so the forwarding itself does not require an SSH server, username, or SSH authentication. The destination service's own authentication still applies.

```sh
subscale mds forward add 100.89.207.112:5900
subscale mds forward add 100.89.207.112:5432 --local 15432
subscale mds forward add 'db.example.ts.net:5432'
subscale mds forward add '[fd7a:115c:a1e0::1]:22'
subscale mds forward list
subscale mds forward remove 100.89.207.112:5900
```

Connect your client to the `127.0.0.1:port` printed by `add`. For screen sharing, use `vnc://127.0.0.1:port` in Finder's Connect to Server dialog. For a database, use host `127.0.0.1` and the displayed port. Forwarded ports are accessible only through `127.0.0.1` on this computer. No browser PAC setting or separate host routing rule is needed. Destination domains are resolved through the selected account's network. Existing `add HOST --to HOST` alias rules do not apply to forwarding destinations.

If you omit `--local`, an available port is assigned automatically and saved. Registering the same account and destination again keeps the existing port. To change a local port, remove the forwarding entry and add it again. You can register the same destination under different accounts with separate local ports. Port `18765` is reserved for the proxy.

Changes take effect immediately while the daemon is running, and the same ports are restored after a restart. To restore them automatically after reboot, register the automatic startup service. If a port is already in use, subscale reports an error in `forward list` and `status` instead of silently choosing another port. Resolve the conflict and retry the same `forward add` command. A `listening` status does not guarantee that the remote device or service is available.

Logging out rejects new connections but keeps forwarding entries. Renaming an account also moves its forwarding entries to the new name. `remove` deletes the forwarding entry and closes ongoing connections. UDP is not supported.

## Address rules

- Domains, single-label MagicDNS names, IPv4, IPv6, and `*.example.com` are supported. CIDR is not supported.
- `*.example.com` matches subdomains only. Register `example.com` separately if needed.
- Exact matches take priority, followed by the most specific wildcard match.
- Registering the same host under another account returns an error. Remove the existing rule first.
- Even if you supply a URL or `host:port`, the rule applies to **all TCP ports for the host**. The URL path, scheme, and port are not stored in the rule.
- Domains are resolved through the selected account's network. If different tailnets have the same `100.x` address, use aliases and `--to` to distinguish them.

```sh
subscale work add work-db.internal --to 100.80.1.2
subscale home add home-db.internal --to 100.80.1.2
subscale exec curl http://work-db.internal:3000
```

`--to` changes only the destination address, keeping the port. For HTTPS, the server certificate must match the alias used to connect. Registering a domain does not make a public internet address use the secondary tailnet as an internet gateway. This version does not provide exit node selection. A subnet address requires a route to that address in the secondary tailnet.

## Saved data and troubleshooting

Sign-in state, address rules, and forwarding entries are stored in the following locations. Set `SUBSCALE_HOME` to use another directory.

- macOS: `~/Library/Application Support/subscale`
- Linux: `$XDG_CONFIG_HOME/subscale` or `~/.config/subscale`

This folder contains account credentials, so do not share it with others. To investigate problems, check `daemon.log` in the folder. Logs may include sign-in instructions, so review them before sharing.

```sh
subscale status
subscale start
subscale stop
```

If another program is using proxy port `18765`, subscale cannot start. Check the conflicting program's port settings.

## Uninstalling

If you enabled automatic proxy settings on macOS, restore the previous settings first, unregister the automatic startup service, and then stop subscale. Replace `Wi-Fi` with the network service name used during setup. Skip the first command if you did not configure automatic proxy settings.

```sh
subscale proxy off Wi-Fi
subscale service uninstall
subscale stop
```

If uninstalling the service already stopped the daemon, `stop` may report that no daemon is running. Then delete the `subscale` file from its installation directory. The default path is `~/.local/bin/subscale`.

Sign-in state and address rules are not deleted automatically. For a complete removal, also delete the data folder listed above.
