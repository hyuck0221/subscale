# 상세 사용법

기존 Tailscale을 그대로 사용하면서, 이름을 붙인 여러 보조 Tailscale 계정으로 지정한 주소에 접근하는 CLI입니다. 계정마다 독립적인 `tsnet` 노드와 인증 저장소를 사용합니다. 시스템 Tailscale의 계정, 인터페이스, DNS, 라우팅 테이블은 변경하지 않습니다.

**현재 지원 범위:** macOS/Linux의 HTTP·HTTPS 프록시와 TCP 터널입니다. 브라우저는 자동 프록시(PAC) 설정으로 등록한 주소만 선택할 수 있습니다. 모든 앱의 패킷을 자동으로 가로채는 VPN/TUN 기능과 UDP/ICMP는 구현하지 않았습니다.

## 설치와 자동 실행

바이너리 설치와 다운로드는 [README](../README.md#설치)를 참고하세요. 소스에서 빌드하려면 Go 1.27.4 이상으로 `make install`을 실행합니다.

`service install`은 macOS에서는 사용자 LaunchAgent, Linux에서는 systemd 사용자 서비스를 설치해 로그인 시 재연결합니다. 설치한 바이너리를 이동하면 서비스를 다시 설치해야 합니다. 로그인 전 부팅 단계의 실행은 지원하지 않습니다. 서비스 설치 없이도 명령을 실행하면 데몬이 자동으로 시작됩니다.

## 계정 등록과 사용

```sh
subscale work login
subscale home login

subscale work add grafana.example.ts.net
subscale work add 100.80.1.2
subscale home add nas.other-tailnet.ts.net
subscale work add '*.corp.example'

subscale status
subscale work list
subscale work remove grafana.example.ts.net  # rm도 가능
subscale work logout
```

계정 이름은 영문자·숫자·`_`·`-`로 1~32자이며, 첫 문자는 영문자 또는 숫자입니다. CLI 명령 이름은 예약되어 있습니다. `login` 시 브라우저에서 **그 이름에 연결할 계정**으로 로그인하세요. 인증 URL도 터미널에 출력됩니다. 서버에서 브라우저를 열지 않으려면 `SUBSCALE_NO_BROWSER=1`을 설정하세요.

인증 상태를 저장하므로 다음 실행에는 다시 로그인하지 않고 연결합니다. 다만 Tailscale 키 만료, 관리자 승인, 기기 삭제, 인증 정책 변경 등으로 재인증이 필요할 수 있습니다. 계정마다 `subscale-work` 같은 별도 기기로 등록되며 tailnet의 기기 제한과 ACL이 적용됩니다. `logout`은 해당 계정만 로그아웃하고 이름과 주소 규칙은 유지합니다. 이후 `login`으로 다시 사용할 수 있습니다.

## 브라우저에서 주소만 입력해 접속

macOS에서는 네트워크 서비스 이름을 지정해 한 번 설정합니다.

```sh
subscale proxy on Wi-Fi
# 유선 네트워크를 사용하는 경우 실제 서비스 이름을 지정하세요.
# networksetup -listallnetworkservices
```

이후 시스템 자동 프록시를 따르는 브라우저에서 등록한 주소에 바로 접속할 수 있습니다. 이전 PAC 설정은 저장되며 다음 명령으로 복구합니다. 네트워크 설정 변경 권한이 필요할 수 있습니다.

```sh
subscale proxy off Wi-Fi
```

다른 OS 또는 별도 프록시 설정을 사용하는 브라우저에는 다음 명령이 출력하는 주소를 **자동 프록시 설정 URL**로 넣으세요.

```sh
subscale proxy pac
# http://127.0.0.1:18765/proxy.pac
```

PAC 규칙은 등록한 호스트에는 `PROXY`, 나머지에는 `DIRECT`를 반환합니다. 규칙은 매 요청에 적용되지만 브라우저의 PAC 캐시 때문에 변경 후 새로고침이나 브라우저 재시작이 필요할 수 있습니다. 보조 계정 오류 시 프록시는 기본 연결로 재시도하지 않습니다. 브라우저 자체의 PAC 다운로드 실패 처리, 로컬 주소 프록시 우회, 기존 연결 재사용까지 강제할 수는 없습니다. 복구가 필요하면 프록시를 끄고 데몬을 재시작하세요.

## 터미널 프로그램과 SSH

```sh
subscale exec curl https://grafana.example.ts.net
subscale exec curl http://100.80.1.2:3000

ssh -o 'ProxyCommand=subscale connect %h %p' user@nas.other-tailnet.ts.net
```

`exec`은 자식 프로세스에 HTTP/HTTPS 프록시 환경변수를 설정하고 `NO_PROXY`를 비웁니다. 프로그램이 이 환경변수를 지원해야 합니다. 임의의 앱이나 UDP 프로그램을 실행한다고 자동으로 우회하지는 않습니다.

반복 사용하는 SSH 연결은 `~/.ssh/config`에 설정할 수 있습니다.

```sshconfig
Host nas.other-tailnet.ts.net
    ProxyCommand subscale connect %h %p
```

`connect`는 표준 입출력을 TCP 연결에 연결합니다. 등록한 주소는 해당 보조 계정으로, 나머지는 OS의 기존 연결로 보냅니다.

## 주소 규칙

- 도메인, 단일 레이블 MagicDNS 이름, IPv4, IPv6, `*.example.com`을 지원합니다. CIDR은 지원하지 않습니다.
- `*.example.com`은 하위 도메인에만 적용됩니다. `example.com` 자체는 별도로 등록해야 합니다.
- 정확히 일치하는 규칙을 우선하고, 그다음 가장 구체적인 와일드카드를 사용합니다.
- 동일 호스트를 다른 계정에 중복 등록하면 오류를 반환합니다. 기존 규칙을 먼저 삭제하세요.
- URL이나 `host:port`를 넣어도 **호스트 전체의 모든 TCP 포트**에 적용합니다. URL의 경로·스킴·포트는 규칙에 저장하지 않습니다.
- 도메인은 선택한 계정의 tsnet에서 해석합니다. 동일한 `100.x` 주소가 서로 다른 tailnet에 있을 때에는 별칭과 `--to`로 구분할 수 있습니다.

```sh
subscale work add work-db.internal --to 100.80.1.2
subscale home add home-db.internal --to 100.80.1.2
subscale exec curl http://work-db.internal:3000
```

`--to`는 연결 대상 호스트만 변경하고 포트, HTTP Host, HTTPS SNI는 유지합니다. HTTPS 서버 인증서가 접속에 사용한 별칭과 일치해야 합니다. 도메인 규칙 등록만으로 공용 인터넷 주소가 보조 tailnet의 출구로 연결되는 것은 아닙니다. 이 버전은 exit node 선택 기능을 제공하지 않습니다. 서브넷 주소는 보조 tailnet에 해당 주소로 갈 수 있는 경로가 있어야 합니다.

## 데이터와 운영

기본 저장 위치는 macOS `~/Library/Application Support/subscale`, Linux `$XDG_CONFIG_HOME/subscale` 또는 `~/.config/subscale`입니다. `SUBSCALE_HOME`으로 덮어쓸 수 있습니다.

```text
config.json           계정 사용 여부와 주소 규칙 (0600, 원자적 교체)
accounts/<name>/      Tailscale 인증과 노드 상태
proxy-backup.json      macOS의 이전 자동 프록시 설정
daemon.sock           로컬 관리 API 소켓 (0600)
daemon.lock           데몬 중복 실행 방지 잠금
daemon.log            데몬 로그와 로그인 안내
```

저장 디렉터리는 0700으로 보호합니다. 인증 상태는 민감한 기기 자격증명이며 앱 자체에서 암호화하지 않습니다. 디렉터리를 공유하거나 저장소에 커밋하지 마세요. 프록시는 `127.0.0.1:18765`에서만 수신하며 로컬 사용자 프로세스가 접근할 수 있습니다. 계정 관리 API는 별도의 사용자 전용 Unix 소켓으로만 제공합니다. 동일 포트에 다른 프로그램이 있으면 데몬 시작이 실패합니다. 로그는 자동 회전하지 않습니다.

```sh
subscale start
subscale stop
subscale service uninstall
```

프록시를 켜 둔 상태로 데몬을 종료하면 등록한 주소의 접속이 실패합니다. 제거할 때는 `proxy off SERVICE`, `service uninstall` 순서로 실행하고 바이너리를 삭제하세요. 자격증명과 규칙은 자동 삭제하지 않습니다.

## 개발과 검증

```sh
make build   # bin/subscale
make test    # Go race detector 포함
make check   # go vet + gofmt
make smoke   # 임시 설정으로 빌드된 CLI 실행 검증 (Python 3, curl 필요)
```

테스트는 주소 정규화·우선순위·계정 충돌, 인증 상태 저장 흐름(대역 구현), 저장 실패 시 규칙 유지, HTTP 프록시 전달, 인증 헤더 제거, HTTPS CONNECT, CONNECT 헤더 뒤에 미리 도착한 데이터, 보조 연결 실패 시 직접 연결 방지를 검증합니다. 실제 여러 계정의 OAuth 인증과 Tailscale 네트워크 간 통신은 본인 계정으로 별도 확인해야 합니다. macOS 네트워크 설정 및 로그인 서비스 설치는 호스트 설정을 변경하므로 자동 테스트에서 실행하지 않습니다.

구조는 `cmd/subscale`(진입점), `internal/cli`(명령·설치), `internal/daemon`(관리 API·프로세스), `internal/config`(저장·규칙), `internal/router`(HTTP·CONNECT·PAC), `internal/tailnet`(tsnet 계정 노드)로 나눕니다.

Tailscale 참고: [tsnet](https://tailscale.com/docs/features/tsnet), [userspace networking](https://tailscale.com/docs/concepts/userspace-networking).

## GitHub 배포

GitHub에서 공개 저장소 `hyuck0221/subscale`을 만든 뒤 소스를 `main` 브랜치에 푸시하세요. `v`로 시작하는 버전 태그를 푸시하면 GitHub Actions가 테스트 후 macOS/Linux × AMD64/ARM64 바이너리와 `checksums.txt`를 Releases에 게시합니다.

```sh
git tag v0.1.0
git push origin v0.1.0
```

첫 릴리스가 게시되면 README의 설치 명령과 [최신 다운로드 링크](https://github.com/hyuck0221/subscale/releases/latest)가 동작합니다. 저장소를 공개해야 비로그인 사용자도 설치할 수 있습니다.

특정 버전이나 설치 경로도 선택할 수 있습니다.

```sh
curl -fsSL https://raw.githubusercontent.com/hyuck0221/subscale/main/scripts/install.sh | SUBSCALE_VERSION=v0.1.0 SUBSCALE_INSTALL_DIR="$HOME/.local/bin" sh
```

직접 릴리스 파일을 빌드하려면 `./scripts/release.sh v0.1.0`을 실행하세요. 결과물은 `dist/`에 생성되며 Git 추적에서 제외됩니다.
