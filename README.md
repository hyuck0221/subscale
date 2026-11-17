# subscale

[한국어](README.md) | [English](README.en.md) | [简体中文](README.zh-CN.md) | [日本語](README.ja.md)

기존 Tailscale 연결을 유지하면서 지정한 주소를 다른 Tailscale 계정으로 연결합니다.

## 설치

### Homebrew

```sh
brew install hyuck0221/tap/subscale
```

### 설치 스크립트

```sh
curl -fsSL https://raw.githubusercontent.com/hyuck0221/subscale/main/scripts/install.sh | sh
```

## 사용법

`[NAME]`은 직접 정하는 계정 이름입니다. 대괄호를 포함한 표시를 실제 값으로 바꿔 입력하세요.

```sh
subscale [NAME] login
subscale [NAME] add [HOST]
subscale [NAME] add '*.example.com' --to [HOST]
subscale [NAME] list
```

| 명령 | 설명 |
| --- | --- |
| `subscale list` | 로그인된 계정 목록 |
| `subscale status` | 전체 계정 상태 |
| `subscale [NAME] rename [NEW_NAME]` | 계정 이름 변경 |
| `subscale [NAME] remove [HOST]` | 주소 규칙 삭제 |
| `subscale [NAME] logout` | 로그아웃 |
| `subscale update` | 최신 버전으로 업데이트 |

## 브라우저 연결

`Wi-Fi`는 사용 중인 macOS 네트워크 서비스 이름으로 바꾸세요.

```sh
subscale proxy on Wi-Fi
subscale proxy off Wi-Fi
```

다른 환경에서는 `subscale proxy pac`가 출력한 URL을 브라우저의 자동 프록시 설정에 입력하세요.

## TCP 포트 포워딩

```sh
subscale [NAME] forward add [HOST]:[PORT]
subscale [NAME] forward add [HOST]:[PORT] --local [LOCAL_PORT]
subscale [NAME] forward list
subscale [NAME] forward remove [HOST]:[PORT]
```

출력된 `127.0.0.1:포트`로 연결하세요. 로컬 포트는 자동 배정·저장되며 데몬 재시작 시 복원됩니다.

## 터미널·SSH

```sh
subscale exec curl https://[HOST]
ssh -o 'ProxyCommand=subscale connect %h %p' [USER]@[HOST]
```

[상세 사용법](docs/usage.md)
