# 상세 사용법

[한국어](usage.md) | [English](usage.en.md) | [简体中文](usage.zh-CN.md) | [日本語](usage.ja.md)

기존 Tailscale을 그대로 사용하면서, 이름을 붙인 여러 보조 Tailscale 계정으로 지정한 주소에 접근하는 CLI입니다. 계정별 로그인 상태를 따로 저장하므로 기존 Tailscale 계정을 전환할 필요가 없습니다.

**현재 지원 범위:** macOS/Linux의 HTTP·HTTPS 프록시와 TCP 터널입니다. 브라우저는 자동 프록시(PAC) 설정으로 등록한 주소만 선택할 수 있습니다. 모든 앱의 패킷을 자동으로 가로채는 VPN/TUN 기능과 UDP/ICMP는 구현하지 않았습니다.

## 설치와 자동 실행

설치 명령은 [README](../README.md#설치)를 참고하세요. 파일을 직접 받으려면 [최신 버전 다운로드](https://github.com/hyuck0221/subscale/releases/latest)에서 OS와 CPU에 맞는 파일을 선택하세요.

기본 설치 경로는 `~/.local/bin`입니다. 설치 후 `subscale` 명령을 찾을 수 없다면 셸 설정 파일에 다음 줄을 추가하세요.

```sh
export PATH="$HOME/.local/bin:$PATH"
```

특정 버전이나 설치 경로를 지정할 수도 있습니다.

```sh
curl -fsSL https://raw.githubusercontent.com/hyuck0221/subscale/main/scripts/install.sh | SUBSCALE_VERSION=v0.1.0 SUBSCALE_INSTALL_DIR="$HOME/.local/bin" sh
```

macOS에서는 설치 스크립트가 자동으로 `subscale service install`을 실행합니다. 기존 설치본은 `subscale service install`을 한 번 실행하면 됩니다. 자동 실행 등록을 생략하려면 설치 스크립트를 실행하는 셸에 `SUBSCALE_AUTOSTART=0`을 설정하세요. 등록에 실패해도 바이너리 설치는 유지되며 재시도 명령을 출력합니다. Linux와 바이너리 수동 다운로드 설치는 `subscale service install`을 직접 실행하세요.

`service install`은 컴퓨터에 로그인할 때 subscale을 실행하고 저장된 계정을 다시 연결합니다. 설치한 바이너리를 이동하면 서비스를 다시 설치해야 합니다. 로그인 전 부팅 단계의 실행은 지원하지 않습니다. 서비스 설치 없이도 명령을 실행하면 데몬이 자동으로 시작됩니다.

## 계정 등록과 사용

```sh
subscale work login
subscale home login

subscale work add grafana.example.ts.net
subscale work add 100.80.1.2
subscale home add nas.other-tailnet.ts.net
subscale work add '*.corp.example'

subscale list                        # 인증된 계정만 표시
subscale status                      # 로그아웃된 계정도 포함
subscale work list
subscale work remove grafana.example.ts.net  # rm도 가능
subscale work logout
```

계정 이름은 영문자·숫자·`_`·`-`로 1~32자이며, 첫 문자는 영문자 또는 숫자입니다. `status`, `list`, `update`, `upgrade`, `rename` 등 CLI 명령과 하위 명령 이름은 대소문자 구분 없이 예약되어 있습니다. `login` 시 브라우저에서 **그 이름에 연결할 계정**으로 로그인하세요. 인증 URL도 터미널에 출력됩니다. 서버에서 브라우저를 열지 않으려면 `SUBSCALE_NO_BROWSER=1`을 설정하세요.

인증 상태를 저장하므로 다음 실행에는 다시 로그인하지 않고 연결합니다. 다만 Tailscale 키 만료, 관리자 승인, 기기 삭제, 인증 정책 변경 등으로 재인증이 필요할 수 있습니다. 계정마다 `subscale-work` 같은 별도 기기로 등록되며 tailnet의 기기 제한과 ACL이 적용됩니다. `logout`은 해당 계정만 로그아웃하고 이름과 주소 규칙은 유지합니다. 이후 `login`으로 다시 사용할 수 있습니다.

계정 이름은 인증이나 주소 규칙을 잃지 않고 변경할 수 있습니다. 기존 이름으로 다시 로그인하면 별개의 새 계정으로 등록됩니다.

```sh
subscale work rename office
subscale office list
```

이미 존재하는 이름이나 명령어 이름으로는 변경할 수 없습니다. 저장에 실패하면 기존 이름과 연결을 유지합니다.

## 기기 조회와 연결 진단

```sh
subscale [NAME] devices
subscale [NAME] check [HOST]:[PORT]
```

`devices`는 해당 계정에서 볼 수 있는 기기의 DNS 이름·IP·온라인 상태·OS를 표시합니다. 현재 subscale 노드는 `(self)`로 표시합니다. 온라인 상태가 서비스 접속 가능 여부를 보장하지는 않습니다.

`check`는 계정 상태, 일치하는 주소 규칙, DNS 해석, TCP 연결을 순서대로 검사합니다. 같은 계정의 `--to` 규칙은 적용하며, 다른 계정의 규칙은 표시만 하고 적용하지 않습니다. 규칙이 없어도 명시한 계정으로 검사할 수 있습니다. 포워딩 대상 자체를 검사하려면 별칭 대신 실제 대상을 입력하세요.

DNS가 선택한 첫 IP에 TCP 연결을 시도하고 즉시 닫습니다. DNS·TCP 단계는 각각 최대 5초이며 실패하면 종료 코드 1을 반환합니다. 결과의 `Network`는 실제 Tailscale 경로 여부를 표시합니다. TCP 성공은 SSH·화면 공유·DB 등의 애플리케이션 인증 성공을 뜻하지 않습니다. 설정은 변경하지 않습니다.

## 업데이트

```sh
subscale update
# 같은 명령: subscale upgrade
```

현재 OS·CPU에 맞는 최신 버전을 내려받아 파일을 검증한 뒤 설치합니다. 이미 최신이면 교체하지 않으며, 다운로드·검증 실패 시 기존 파일을 유지합니다. 설치 디렉터리에 쓰기 권한이 필요합니다.

실행 중인 데몬은 저장한 계정과 규칙을 유지한 채 재시작합니다. 이때 기존 프록시·SSH 연결은 끊길 수 있습니다.

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

등록한 주소는 지정한 보조 계정으로, 나머지 주소는 기존 연결로 접속합니다. 브라우저의 자동 프록시 설정이 캐시될 수 있으므로 규칙 변경 후 새로고침이나 브라우저 재시작이 필요할 수 있습니다. 보조 계정 오류 시 프록시는 기본 연결로 재시도하지 않습니다. 브라우저 자체의 PAC 다운로드 실패 처리, 로컬 주소 프록시 우회, 기존 연결 재사용까지 강제할 수는 없습니다. 복구가 필요하면 프록시를 끄고 데몬을 재시작하세요.

## 터미널 프로그램과 SSH

```sh
subscale exec curl https://grafana.example.ts.net
subscale exec curl http://100.80.1.2:3000

ssh -o 'ProxyCommand=subscale connect %h %p' user@nas.other-tailnet.ts.net
```

`exec`으로 실행한 프로그램은 HTTP/HTTPS 프록시 환경변수를 지원해야 합니다. 임의의 앱이나 UDP 프로그램을 실행한다고 자동으로 우회하지는 않습니다.

반복 사용하는 SSH 연결은 `~/.ssh/config`에 설정할 수 있습니다.

```sshconfig
Host nas.other-tailnet.ts.net
    ProxyCommand subscale connect %h %p
```

`connect`를 SSH의 `ProxyCommand`로 지정하면 등록한 주소는 해당 보조 계정으로, 나머지는 기존 연결로 접속합니다.

## 로컬 TCP 포트 포워딩

화면 공유, SSH, DB 등 TCP 서비스에 사용할 수 있습니다. SSH 터널링이 아니라 선택한 계정의 Tailscale 연결을 통한 TCP 중계이므로 SSH 서버·사용자명·SSH 인증이 필요하지 않습니다. 접속한 서비스 자체의 인증은 그대로 적용됩니다.

```sh
subscale mds forward add 100.89.207.112:5900
subscale mds forward add 100.89.207.112:5432 --local 15432
subscale mds forward add 'db.example.ts.net:5432'
subscale mds forward add '[fd7a:115c:a1e0::1]:22'
subscale mds forward list
subscale mds forward remove 100.89.207.112:5900
```

`add`가 출력한 `127.0.0.1:포트`로 클라이언트를 연결하세요. 화면 공유라면 Finder의 서버 연결에 `vnc://127.0.0.1:포트`, DB라면 호스트 `127.0.0.1`과 해당 포트를 사용합니다. 포워딩 포트는 이 컴퓨터의 `127.0.0.1`에서만 접속할 수 있습니다. 브라우저 PAC 설정이나 별도의 호스트 라우팅 규칙 등록은 필요하지 않으며, 대상 도메인은 선택한 계정의 네트워크에서 해석합니다. 기존 `add HOST --to HOST` 별칭 규칙은 포워딩 대상에 적용되지 않습니다.

`--local`을 생략하면 사용 가능한 포트를 자동 배정해 저장합니다. 동일 계정·대상을 다시 등록하면 기존 포트를 유지합니다. 로컬 포트를 바꾸려면 삭제 후 다시 등록하세요. 같은 대상도 다른 계정에 별도 로컬 포트로 등록할 수 있습니다. 프록시용 포트 `18765`는 예약되어 있습니다.

등록은 데몬 실행 중 즉시 반영되며, 재시작하면 같은 포트를 복원합니다. 재부팅 후 자동 복원하려면 로그인 자동 실행 서비스가 등록되어 있어야 합니다. 포트가 이미 사용 중이면 다른 포트로 임의 변경하지 않고 `forward list`와 `status`에 오류를 표시합니다. 충돌을 해결한 뒤 같은 `forward add` 명령으로 재시도할 수 있습니다. `listening`이 표시되어도 원격 기기나 서비스가 꺼져 있으면 접속할 수 없습니다.

로그아웃하면 새 연결을 거부하고 등록은 유지합니다. 계정 이름 변경 시 포워딩도 함께 이동합니다. `remove`는 포워딩 등록을 삭제하고 진행 중인 연결도 종료합니다. UDP는 지원하지 않습니다.

## 주소 규칙

- 도메인, 단일 레이블 MagicDNS 이름, IPv4, IPv6, `*.example.com`을 지원합니다. CIDR은 지원하지 않습니다.
- `*.example.com`은 하위 도메인에만 적용됩니다. `example.com` 자체는 별도로 등록해야 합니다.
- 정확히 일치하는 규칙을 우선하고, 그다음 가장 구체적인 와일드카드를 사용합니다.
- 동일 호스트를 다른 계정에 중복 등록하면 오류를 반환합니다. 기존 규칙을 먼저 삭제하세요.
- URL이나 `host:port`를 넣어도 **호스트 전체의 모든 TCP 포트**에 적용합니다. URL의 경로·스킴·포트는 규칙에 저장하지 않습니다.
- 도메인은 선택한 계정의 네트워크에서 해석합니다. 동일한 `100.x` 주소가 서로 다른 tailnet에 있을 때에는 별칭과 `--to`로 구분할 수 있습니다.

```sh
subscale work add work-db.internal --to 100.80.1.2
subscale home add home-db.internal --to 100.80.1.2
subscale exec curl http://work-db.internal:3000
```

`--to`는 연결 대상 주소만 바꾸고 포트는 유지합니다. HTTPS로 접속할 때는 서버 인증서가 접속에 사용한 별칭과 일치해야 합니다. 도메인 규칙 등록만으로 공용 인터넷 주소가 보조 tailnet의 출구로 연결되는 것은 아닙니다. 이 버전은 exit node 선택 기능을 제공하지 않습니다. 서브넷 주소는 보조 tailnet에 해당 주소로 갈 수 있는 경로가 있어야 합니다.

## 저장된 데이터와 문제 해결

로그인 상태와 주소·포워딩 규칙은 다음 위치에 저장합니다. `SUBSCALE_HOME`으로 저장 위치를 바꿀 수 있습니다.

- macOS: `~/Library/Application Support/subscale`
- Linux: `$XDG_CONFIG_HOME/subscale` 또는 `~/.config/subscale`

이 폴더에는 계정 인증 정보가 포함되므로 다른 사람에게 공유하지 마세요. 문제를 확인하려면 폴더 안의 `daemon.log`를 참고하세요. 로그에도 로그인 안내가 포함될 수 있으므로 공유 전 내용을 확인하세요.

```sh
subscale status
subscale start
subscale stop
```

프록시용 포트 `18765`를 다른 프로그램이 사용하고 있으면 subscale을 시작할 수 없습니다. 충돌하는 프로그램의 포트 설정을 확인하세요.

## 제거

macOS에서 자동 프록시를 켰다면 먼저 이전 설정을 복구하고, 자동 실행 서비스를 해제한 뒤 subscale을 종료하세요. `Wi-Fi`는 설정할 때 사용한 네트워크 서비스 이름으로 바꾸세요. 자동 프록시를 설정하지 않았다면 첫 명령은 생략합니다.

```sh
subscale proxy off Wi-Fi
subscale service uninstall
subscale stop
```

서비스 해제 과정에서 이미 종료되었다면 `stop` 명령이 실행 중인 데몬이 없다고 표시할 수 있습니다. 이후 설치 경로의 `subscale` 파일을 삭제하세요. 기본 경로는 `~/.local/bin/subscale`입니다.

로그인 상태와 주소 규칙은 자동 삭제하지 않습니다. 완전히 제거하려면 위의 데이터 저장 폴더도 삭제하세요.
