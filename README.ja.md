# subscale

[한국어](README.md) | [English](README.en.md) | [简体中文](README.zh-CN.md) | [日本語](README.ja.md)

既存の Tailscale 接続を維持したまま、指定したアドレスに別の Tailscale アカウントで接続します。

## インストール

### Homebrew

```sh
brew install hyuck0221/tap/subscale
```

### インストールスクリプト

```sh
curl -fsSL https://raw.githubusercontent.com/hyuck0221/subscale/main/scripts/install.sh | sh
```

## 使い方

`[NAME]` は自分で決めるアカウント名です。角括弧を含むプレースホルダー全体を実際の値に置き換えて入力してください。

```sh
subscale [NAME] login
subscale [NAME] add [HOST]
subscale [NAME] add '*.example.com' --to [HOST]
subscale [NAME] list
```

| コマンド | 説明 |
| --- | --- |
| `subscale list` | ログイン済みアカウントの一覧 |
| `subscale status` | すべてのアカウントの状態を表示 |
| `subscale [NAME] rename [NEW_NAME]` | アカウント名を変更 |
| `subscale [NAME] remove [HOST]` | アドレスルールを削除 |
| `subscale [NAME] logout` | ログアウト |
| `subscale update` | 最新バージョンに更新 |

## ブラウザーでの接続

`Wi-Fi` は使用中の macOS ネットワークサービス名に置き換えてください。

```sh
subscale proxy on Wi-Fi
subscale proxy off Wi-Fi
```

それ以外の環境では、`subscale proxy pac` が出力する URL をブラウザーの自動プロキシ設定に入力してください。

## TCP ポート転送

```sh
subscale [NAME] forward add [HOST]:[PORT]
subscale [NAME] forward add [HOST]:[PORT] --local [LOCAL_PORT]
subscale [NAME] forward list
subscale [NAME] forward remove [HOST]:[PORT]
```

表示された `127.0.0.1:ポート` に接続してください。ローカルポートは自動で割り当て・保存され、デーモンの再起動時に復元されます。

## ターミナル・SSH

```sh
subscale exec curl https://[HOST]
ssh -o 'ProxyCommand=subscale connect %h %p' [USER]@[HOST]
```

[詳しい使い方](docs/usage.ja.md)
