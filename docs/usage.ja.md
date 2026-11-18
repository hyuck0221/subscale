# 詳しい使い方

[한국어](usage.md) | [English](usage.en.md) | [简体中文](usage.zh-CN.md) | [日本語](usage.ja.md)

subscale は、既存の Tailscale 接続を維持しながら、名前を付けた複数の補助 Tailscale アカウントで指定したアドレスに接続する CLI です。ログイン状態はアカウントごとに保存されるため、既存の Tailscale アカウントを切り替える必要はありません。

**対応範囲：** macOS/Linux の HTTP・HTTPS プロキシと TCP トンネルに対応しています。ブラウザーでは自動プロキシ設定（PAC）により、登録したアドレスだけ補助接続を利用できます。すべてのアプリに透過的に適用される VPN/TUN ルーティングや UDP/ICMP には対応していません。

## インストールと自動起動

インストールコマンドは [README](../README.ja.md#インストール) を参照してください。手動でダウンロードする場合は、[最新バージョンのダウンロードページ](https://github.com/hyuck0221/subscale/releases/latest)で OS と CPU に合ったファイルを選択してください。

既定のインストール先は `~/.local/bin` です。インストール後に `subscale` コマンドが見つからない場合は、シェルの設定ファイルに次の行を追加してください。

```sh
export PATH="$HOME/.local/bin:$PATH"
```

バージョンやインストール先を指定することもできます。

```sh
curl -fsSL https://raw.githubusercontent.com/hyuck0221/subscale/main/scripts/install.sh | SUBSCALE_VERSION=v0.1.0 SUBSCALE_INSTALL_DIR="$HOME/.local/bin" sh
```

macOS ではインストールスクリプトが自動で `subscale service install` を実行します。既存のインストールでは、一度 `subscale service install` を実行してください。自動起動の登録を省略するには、インストールスクリプトを実行するシェルで `SUBSCALE_AUTOSTART=0` を設定します。登録に失敗しても実行ファイルはインストールされたままで、再試行用のコマンドが表示されます。Linux または実行ファイルを手動でダウンロードした場合は、`subscale service install` を自分で実行してください。

`service install` は、パソコンへのログイン時に subscale を起動し、保存済みのアカウントに再接続します。インストールした実行ファイルを移動した場合は、サービスを再インストールしてください。ユーザーログイン前のシステム起動段階での実行には対応していません。サービスをインストールしていなくても、コマンドを実行するとデーモンは自動で起動します。

## アカウントの登録と使用

```sh
subscale work login
subscale home login

subscale work add grafana.example.ts.net
subscale work add 100.80.1.2
subscale home add nas.other-tailnet.ts.net
subscale work add '*.corp.example'

subscale list                        # 認証済みアカウントのみ表示
subscale status                      # ログアウト済みアカウントも含む
subscale work list
subscale work remove grafana.example.ts.net  # rm も使用可能
subscale work clear
subscale work logout
```

アカウント名は英字・数字・`_`・`-` の 1～32 文字で、先頭は英字または数字にしてください。`status`、`list`、`update`、`upgrade`、`rename` などのコマンド名とサブコマンド名は、大文字・小文字を問わず予約されています。`login` 時には、ブラウザーで**その名前に関連付けたい Tailscale アカウント**にログインしてください。認証 URL はターミナルにも表示されます。サーバー上でブラウザーを開きたくない場合は、`SUBSCALE_NO_BROWSER=1` を設定してください。

各アカウントは `subscale-work` のような別のデバイスとして登録され、tailnet のデバイス数制限と ACL が適用されます。新しいアカウントは一時デバイスとして登録されます。デーモンの終了時や一定時間オフラインの状態が続いた場合に、Tailscale がデバイスを削除することがあります。その場合は `login` で再認証してください。

`clear` はそのアカウントのアドレスルールと TCP 転送をすべて削除し、ログイン状態を保持します。ログアウト済みのアカウントにも使用できます。`logout` はローカル設定を削除し、Tailscale の Machines からデバイスを削除した後、ローカルのアカウント名と認証情報も削除します。どちらのコマンドも確認プロンプトで `y` を入力すると実行されます。

一時デバイスの `logout` には Tailscale のデバイスログアウト機能を使います。リモート側のログアウトに失敗した場合は、同じコマンドを再実行できます。以前のバージョンで通常のデバイスとして登録したアカウントは、Machines でデバイスを削除してから `subscale work logout --machine-removed` でローカルの削除を完了してください。

ログイン状態やアドレスルールを失わずにアカウント名を変更できます。変更前の名前で再度ログインすると、別の新しいアカウントとして登録されます。

```sh
subscale work rename office
subscale office list
```

既存のアカウント名やコマンド名には変更できません。保存に失敗した場合は、元の名前と接続を維持します。

## デバイス一覧と接続診断

```sh
subscale [NAME] devices
subscale [NAME] check [HOST]:[PORT]
```

`devices` はそのアカウントから見えるデバイスの DNS 名、IP、オンライン状態、OS を表示します。現在の subscale ノードは `(self)` と表示されます。オンラインであってもサービスに接続できるとは限りません。

`check` はアカウント状態、アドレスルール、名前解決、TCP 接続を順に診断します。同じアカウントの `--to` ルールを適用し、別アカウントのルールは表示のみ行います。ルールの事前登録は不要です。ポート転送先自体を確認する場合は、別名ではなく実際の転送先を入力してください。

名前解決で最初に得られた IP に TCP 接続し、直ちに閉じます。DNS と TCP のタイムアウトはそれぞれ 5 秒で、失敗時の終了コードは 1 です。`Network` は Tailscale 経路を使用するかを示します。TCP 接続の成功は SSH、画面共有、DB の認証成功を意味しません。設定は変更しません。

## 更新

```sh
subscale update
# 同じ動作のコマンド：subscale upgrade
```

現在の OS・CPU に合った最新バージョンをダウンロードし、ファイルを検証してからインストールします。すでに最新の場合は置き換えません。ダウンロードや検証に失敗した場合は既存のファイルを維持します。インストール先への書き込み権限が必要です。

実行中のデーモンは、保存済みのアカウントとルールを維持したまま再起動します。このとき、既存のプロキシ接続や SSH 接続が切断される場合があります。

## ブラウザーでアドレスを直接開く

macOS ではネットワークサービス名を指定して、一度設定します。

```sh
subscale proxy on Wi-Fi
# 有線ネットワークの場合は、実際のサービス名を指定してください。
# networksetup -listallnetworkservices
```

以降は、システムの自動プロキシ設定に従うブラウザーで、登録したアドレスに直接アクセスできます。以前の PAC 設定は保存され、次のコマンドで復元できます。ネットワーク設定の変更権限が必要な場合があります。

```sh
subscale proxy off Wi-Fi
```

ほかの OS や独自のプロキシ設定を使うブラウザーでは、次のコマンドが出力するアドレスを**自動プロキシ設定 URL**として入力してください。

```sh
subscale proxy pac
# http://127.0.0.1:18765/proxy.pac
```

登録したアドレスは指定した補助アカウントで、それ以外のアドレスは既存の接続でアクセスします。ブラウザーが自動プロキシ設定をキャッシュする場合があるため、ルール変更後にページの再読み込みやブラウザーの再起動が必要になることがあります。補助アカウントでエラーが発生した場合、プロキシは既定の接続で再試行しません。ただし、PAC のダウンロードに失敗したときのブラウザーの動作、ローカルアドレスでのプロキシ回避、既存の接続の再利用までは制御できません。復旧が必要な場合は、プロキシを無効にしてデーモンを再起動してください。

## ターミナルプログラムと SSH

```sh
subscale exec curl https://grafana.example.ts.net
subscale exec curl http://100.80.1.2:3000

ssh -o 'ProxyCommand=subscale connect %h %p' user@nas.other-tailnet.ts.net
```

`exec` で起動するプログラムは、HTTP/HTTPS プロキシ環境変数に対応している必要があります。任意のアプリや UDP プログラムを起動しても、自動で subscale を経由するわけではありません。

よく使う SSH 接続は `~/.ssh/config` に設定できます。

```sshconfig
Host nas.other-tailnet.ts.net
    ProxyCommand subscale connect %h %p
```

SSH の `ProxyCommand` に `connect` を指定すると、登録したアドレスは対応する補助アカウントで、それ以外は既存の接続でアクセスします。

## ローカル TCP ポート転送

画面共有、SSH、データベースなどの TCP サービスに使用できます。転送は選択したアカウントの Tailscale 接続を通じて TCP 通信を中継するため、転送自体に SSH サーバー・ユーザー名・SSH 認証は必要ありません。接続先のサービス自体の認証は引き続き必要です。

以下の例では、`[NAME]` を登録済みのアカウント名、`[HOST]` を接続先デバイスのアドレスに置き換えてください。

```sh
subscale [NAME] forward add [HOST]:5900
subscale [NAME] forward add [HOST]:5432 --local 15432
subscale [NAME] forward add 'db.example.ts.net:5432'
subscale [NAME] forward add '[fd7a:115c:a1e0::1]:22'
subscale [NAME] forward list
subscale [NAME] forward remove [HOST]:5900
```

`add` が出力する `127.0.0.1:ポート` にクライアントを接続してください。画面共有では Finder の「サーバへ接続」に `vnc://127.0.0.1:ポート` を入力し、データベースではホスト `127.0.0.1` と表示されたポートを指定します。転送ポートには、このパソコンの `127.0.0.1` からのみ接続できます。ブラウザーの PAC 設定や個別のホストルーティングルールは不要です。接続先のドメインは選択したアカウントのネットワークで名前解決されます。既存の `add HOST --to HOST` の別名ルールは、転送先には適用されません。

`--local` を省略すると、利用可能なポートを自動で割り当てて保存します。同じアカウントと転送先を再登録しても、既存のポートを維持します。ローカルポートを変更するには、削除してから再登録してください。同じ転送先を別のアカウントに登録し、それぞれ異なるローカルポートを使うこともできます。ポート `18765` はプロキシ用に予約されています。

登録内容はデーモンの実行中にすぐ反映され、再起動後も同じポートが復元されます。パソコンの再起動後に自動で復元するには、ログイン時の自動起動サービスを登録してください。ポートがすでに使用中の場合は、勝手に別のポートに変更せず、`forward list` と `status` にエラーを表示します。競合を解消した後、同じ `forward add` コマンドで再試行できます。`listening` と表示されていても、リモートのデバイスやサービスが停止していると接続できません。

`clear` と `logout` はそのアカウントの TCP 転送をすべて削除し、接続中の通信も終了します。アカウント名を変更すると、転送の登録も新しい名前に引き継がれます。`remove` は転送の登録を削除し、接続中の通信も終了します。UDP には対応していません。

## アドレスルール

- ドメイン、単一ラベルの MagicDNS 名、IPv4、IPv6、`*.example.com` に対応しています。CIDR には対応していません。
- `*.example.com` はサブドメインだけに適用されます。`example.com` 自体は個別に登録してください。
- 完全一致のルールを優先し、次に最も具体的なワイルドカードを使用します。
- 同じホストを別のアカウントに重複登録するとエラーになります。既存のルールを先に削除してください。
- URL や `host:port` を入力しても、ルールは**ホストのすべての TCP ポート**に適用されます。URL のパス・スキーム・ポートはルールに保存されません。
- ドメインは選択したアカウントのネットワークで名前解決されます。異なる tailnet に同じ `100.x` アドレスがある場合は、別名と `--to` で区別できます。

```sh
subscale work add work-db.internal --to 100.80.1.2
subscale home add home-db.internal --to 100.80.1.2
subscale exec curl http://work-db.internal:3000
```

`--to` は接続先のアドレスだけを変更し、ポートは維持します。HTTPS で接続する場合、サーバー証明書が接続に使う別名と一致している必要があります。ドメインを登録するだけでは、公開インターネットのアドレスへのアクセスが補助 tailnet を出口として行われるようにはなりません。このバージョンには出口ノード（exit node）を選択する機能はありません。サブネットのアドレスに接続するには、補助 tailnet にそのアドレスへのルートが必要です。

## 保存データとトラブルシューティング

ログイン状態、アドレスルール、転送の登録は次の場所に保存されます。`SUBSCALE_HOME` で保存先を変更できます。

- macOS: `~/Library/Application Support/subscale`
- Linux: `$XDG_CONFIG_HOME/subscale` または `~/.config/subscale`

このフォルダーにはアカウントの認証情報が含まれるため、他人と共有しないでください。問題の調査には、フォルダー内の `daemon.log` を確認してください。ログにもログインの案内が含まれる場合があるため、共有前に内容を確認してください。

```sh
subscale status
subscale start
subscale stop
```

ほかのプログラムがプロキシ用ポート `18765` を使用していると、subscale を起動できません。競合するプログラムのポート設定を確認してください。

## アンインストール

macOS で自動プロキシを有効にした場合は、先に以前の設定を復元し、自動起動サービスを解除してから subscale を停止してください。`Wi-Fi` は設定時に使用したネットワークサービス名に置き換えます。自動プロキシを設定していない場合は、最初のコマンドを省略してください。

```sh
subscale proxy off Wi-Fi
subscale service uninstall
subscale stop
```

サービスの解除時にすでにデーモンが停止していれば、`stop` で実行中のデーモンがないと表示される場合があります。その後、インストール先の `subscale` ファイルを削除してください。既定のパスは `~/.local/bin/subscale` です。

ログイン状態とアドレスルールは自動では削除されません。完全に削除する場合は、上記のデータ保存フォルダーも削除してください。
