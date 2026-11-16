# subscale

기존 Tailscale 연결을 유지하면서, 지정한 주소만 다른 Tailscale 계정으로 연결하는 CLI입니다. 여러 계정을 이름으로 등록하고, 한 번 인증한 상태를 저장해 재사용합니다.

## 설치

macOS·Linux에서 Go 설치 없이 사용할 수 있습니다.

```sh
curl -fsSL https://raw.githubusercontent.com/hyuck0221/subscale/main/scripts/install.sh | sh
```

기본 설치 경로는 `~/.local/bin`입니다. PATH에 없다면 셸 설정 파일에 추가하세요.

macOS에서는 설치 스크립트가 로그인 시 자동 실행을 함께 등록합니다. 재부팅 후 로그인하면 저장한 계정과 주소 규칙으로 자동 재연결합니다.

```sh
export PATH="$HOME/.local/bin:$PATH"
```

[GitHub Releases에서 직접 다운로드](https://github.com/hyuck0221/subscale/releases/latest)할 수도 있습니다. macOS·Linux의 ARM64·AMD64 바이너리를 제공합니다.

## 빠른 시작

```sh
subscale work login                      # 브라우저에서 work용 계정 인증
subscale work add nas.example.ts.net      # 이 주소를 work 계정으로 연결
subscale work add 100.80.1.2

subscale home login                      # 다른 계정도 함께 등록
subscale home add '*.home.example'

subscale list                            # 로그인된 계정 목록
```

브라우저에서 자동으로 연결하려면 macOS에서 한 번 설정합니다. `Wi-Fi`는 사용 중인 네트워크 서비스 이름으로 바꾸세요.

```sh
subscale proxy on Wi-Fi
```

다른 OS나 별도 프록시 설정을 쓰는 브라우저에서는 `subscale proxy pac`가 출력하는 URL을 자동 프록시 설정에 넣으세요.

터미널과 SSH에서도 사용할 수 있습니다.

```sh
subscale exec curl https://nas.example.ts.net
ssh -o 'ProxyCommand=subscale connect %h %p' user@nas.example.ts.net
```

## 자주 쓰는 명령

| 명령 | 설명 |
| --- | --- |
| `subscale list` | 로그인된 모든 계정 확인 |
| `subscale update` / `subscale upgrade` | 최신 버전으로 업데이트 |
| `subscale work rename office` | 인증·주소 규칙을 유지하며 이름 변경 |
| `subscale work login` | 계정 등록·로그인 |
| `subscale work logout` | 해당 계정 로그아웃, 주소 규칙 유지 |
| `subscale work add 주소` | 도메인·IP·와일드카드 등록 |
| `subscale work remove 주소` | 주소 규칙 삭제 |
| `subscale work list` | 계정 상태와 주소 규칙 확인 |
| `subscale service install` | 로그인 시 자동 실행 |
| `subscale proxy off Wi-Fi` | 이전 자동 프록시 설정 복구 |

`status`, `list`, `update` 등 명령어 이름은 계정 이름으로 사용할 수 없습니다. 로그아웃된 계정까지 보려면 `subscale status`를 사용하세요.

## 지원 범위

- HTTP/HTTPS 프록시와 SSH/TCP 터널을 지원합니다. 모든 앱의 투명 라우팅과 UDP/ICMP는 지원하지 않습니다.
- 등록하지 않은 주소는 기존 연결을 사용합니다. `exec`은 프록시 환경변수를 지원하는 프로그램에서 동작합니다.
- 인증은 저장되지만 Tailscale 키 만료나 관리자 정책에 따라 재인증이 필요할 수 있습니다.

주소 별칭(`--to`), 데이터 저장 위치, 제거 방법, 개발·테스트는 [상세 사용법](docs/usage.md)을 참고하세요.
