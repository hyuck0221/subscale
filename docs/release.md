# 릴리스와 Homebrew Tap 자동 배포

`vMAJOR.MINOR.PATCH` 태그를 푸시하면 Release 워크플로가 순서대로 실행됩니다.

1. 테스트 후 macOS/Linux × ARM64/AMD64 바이너리와 `checksums.txt` 생성
2. GitHub Release 게시
3. 게시된 릴리스의 `checksums.txt` 다운로드
4. `hyuck0221/homebrew-tap`의 네 플랫폼 URL·체크섬 갱신
5. Tap 기본 브랜치에 `subscale v버전` 커밋·푸시

## 최초 인증 설정

기본 `GITHUB_TOKEN`은 subscale 저장소에만 권한이 있으므로 Tap 쓰기용 토큰이 필요합니다.

1. [Fine-grained personal access token 생성](https://github.com/settings/personal-access-tokens/new)
   - Resource owner: `hyuck0221`
   - Repository access: **Only select repositories** → `homebrew-tap`
   - Repository permissions: **Contents → Read and write**
2. [subscale Actions secrets](https://github.com/hyuck0221/subscale/settings/secrets/actions)에서 **New repository secret** 선택
   - Name: `HOMEBREW_TAP_TOKEN`
   - Secret: 생성한 토큰

Tap 기본 브랜치에 해당 토큰으로 직접 푸시할 수 있어야 합니다. 토큰이 만료되면 같은 secret을 새 토큰으로 갱신하세요. 토큰 값을 코드나 채팅에 넣지 마세요.

## 배포

워크플로와 스크립트 변경을 먼저 커밋·푸시한 뒤 새 태그를 생성하세요. 기존 태그를 이동시키지 않습니다.

```sh
git tag v0.1.4
git push origin v0.1.4
```

`v0.1.4`는 예시이며 다음 릴리스 버전으로 바꾸세요. `-rc.1` 같은 prerelease 태그와 `+build` 태그는 Tap을 갱신하지 않습니다. 늦게 끝난 과거 릴리스가 Tap 버전을 내리지 않으며, 동일 작업을 다시 실행해 변경이 없으면 추가 커밋을 만들지 않습니다.

사용자는 다음 명령으로 갱신합니다.

```sh
brew update
brew upgrade hyuck0221/tap/subscale
brew services restart hyuck0221/tap/subscale
```

## Tap 갱신 실패 시

`update-tap` 실패는 이미 게시된 Release를 취소하지 않습니다. 토큰·브랜치 권한·Formula 형식 등을 수정한 뒤 해당 Actions 실행에서 **Re-run failed jobs**를 실행하세요. 토큰만 추가·갱신한 경우에도 같은 방식으로 재시도할 수 있습니다.

Formula의 플랫폼 블록 형식이 바뀌면 안전하게 실패하므로 `scripts/update_tap.py`도 함께 수정하세요. 스크립트 코드 수정이 필요한 경우에는 수정 사항을 포함한 새 태그로 배포합니다.
