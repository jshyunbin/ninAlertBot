# ninAlertBot

[English](README.md) | **한국어**

**닌텐도 코리아 스토어**(`store.nintendo.co.kr`)를 감시하다가, 상품이 품절에서
구매 가능 상태로 바뀌는 순간 **디스코드**로 메시지를 보내주는 가벼운 백그라운드
서비스입니다.

- **단일 실행 파일**, 약 9 MB, 대기 시 메모리 약 10–15 MB — 24시간 상시 구동용으로 제작.
- **크로스 플랫폼** (Windows / Linux / macOS).
- **자유로운 설정** — 원하는 만큼 상품 등록 가능 (일반 Switch 2, 포코피아 번들,
  마리오카트 월드 세트, 주변기기 등).
- **스팸 없음** — 재입고당 한 번만 알림, 재시작 후에도 상태를 기억합니다.

## 동작 방식

각 상품은 `https://store.nintendo.co.kr/<slug>` 페이지에서 재고 상태를 서버에서
렌더링합니다:

| 상태      | 마크업                                  |
|-----------|-----------------------------------------|
| 품절      | `<div class="stock unavailable">품절</div>`     |
| 구매 가능 | `<div class="stock available">구매 가능</div>`  |

ninAlertBot은 설정된 각 상품을 일정 주기로 확인하고, **품절 → 구매 가능** 전환이
일어나면 디스코드 웹훅을 발송합니다.

## 설치

### 1. 디스코드 웹훅 만들기
디스코드 서버에서: **서버 설정 → 연동 → 웹후크 → 새 웹후크**, 채널을 선택한 뒤
**웹후크 URL 복사**.

### 2. 설정
```bash
cp config.example.yaml config.yaml
# config.yaml 편집: 웹훅 URL을 붙여넣고 감시할 상품을 선택
```

상품의 `slug`는 스토어에서 해당 상품 페이지를 열면 URL의 마지막 부분입니다.
알려진 Switch 2 슬러그:

| 상품                                        | Slug           |
|---------------------------------------------|----------------|
| Nintendo Switch 2                           | `beeskb6aakor` |
| Nintendo Switch 2 + 포켓몬 포코피아 세트     | `beeskb6nfkor` |
| Nintendo Switch 2 + 마리오카트 월드 세트     | `beeskb6nakor` |

### 3. 빌드
Go 1.24+ 가 설치된 경우:
```bash
go build -o ninalertbot ./cmd/ninalertbot          # 현재 플랫폼
GOOS=windows GOARCH=amd64 go build -o ninalertbot.exe ./cmd/ninalertbot   # Windows
```

> 빌드 없이 바로 쓰려면 [릴리스 페이지](https://github.com/jshyunbin/ninAlertBot/releases)
> 에서 플랫폼에 맞는 zip을 받으세요 (Go 설치 불필요).

### 4. 실행
```bash
./ninalertbot -config config.yaml          # 계속 실행
./ninalertbot -config config.yaml -once    # 한 번만 확인 후 종료 (테스트)
./ninalertbot -config config.yaml -debug   # 상세 로그
```

## Windows에서 24시간 상시 구동

### 방법 A — 작업 스케줄러 (가장 간단)
1. `ninalertbot.exe`와 `config.yaml`을 예를 들어 `C:\ninAlertBot\`에 넣습니다.
2. **작업 스케줄러 → 작업 만들기**를 엽니다.
   - **일반:** "사용자의 로그온 여부에 관계없이 실행", "가장 높은 수준의 권한으로 실행" 체크.
   - **트리거:** 새로 만들기 → "시작할 때".
   - **동작:** 새로 만들기 → 프로그램: `C:\ninAlertBot\ninalertbot.exe`
     인수 추가: `-config config.yaml`
     시작 위치: `C:\ninAlertBot\`
   - **설정:** "작업이 실패하면 다시 시작 간격 1분"; "다음 시간보다 오래 실행되면 작업 중지" 체크 해제.
3. 작업을 우클릭 → **실행**.

### 방법 B — NSSM으로 Windows 서비스 등록
```powershell
nssm install ninAlertBot C:\ninAlertBot\ninalertbot.exe -config config.yaml
nssm set ninAlertBot AppDirectory C:\ninAlertBot
nssm start ninAlertBot
```

## 설정 항목 참고

[`config.example.yaml`](config.example.yaml) 참고. 주요 항목:

| 항목                      | 기본값  | 설명                                                  |
|---------------------------|---------|-------------------------------------------------------|
| `discord_webhook_url`     | —       | 필수. 디스코드 수신 웹훅 (https).                     |
| `interval`                | `60s`   | 확인 주기, ±20% 무작위 편차. 최소 `10s`.              |
| `mention`                 | —       | 기본 멘션. 상품에 `mentions`가 없을 때 사용. 예: `@here` 또는 `<@USER_ID>`. |
| `renotify_after`          | `0s`    | 계속 구매 가능 시 이 시간 후 재알림. 0 = 한 번만.     |
| `notify_on_scraper_break` | `false` | 페이지 파싱 실패 시 진단용 알림 발송.                 |
| `products[].name` / `.slug` | —     | 상품의 표시 이름과 URL 슬러그.                        |
| `products[].mentions`     | —       | 선택. 해당 상품에 한해 전역 `mention`을 **대체**하는 멘션 목록. |

상태는 실행 파일 옆 `state.json`에 저장됩니다 (`-state`로 경로 변경 가능).

> **참고:** 알림 메시지는 **한국어**로 전송됩니다 (예: "🟢 **Nintendo Switch 2**
> 지금 구매 가능합니다!").

### 상품별 멘션 타게팅

각 상품에 `mentions` 목록을 지정해 상품마다 다른 사람을 호출할 수 있습니다.
사용자 ID(`<@123>`)와 역할 ID(`<@&456>`)를 모두 받을 수 있으며, 지정되면 해당
상품에 한해 전역 `mention`을 덮어씁니다:

```yaml
mention: "@here"            # mentions가 없는 상품의 기본값
products:
  - name: "Nintendo Switch 2"
    slug: "beeskb6aakor"
    mentions: ["<@B_USER_ID>"]                 # B는 Switch 2만 알림
  - name: "Nintendo Switch 2 + 포켓몬 포코피아 세트"
    slug: "beeskb6nfkor"
    mentions: ["<@A_USER_ID>"]                 # A는 번들 알림
  - name: "Nintendo Switch 2 + 마리오카트 월드 세트"
    slug: "beeskb6nakor"
    mentions: ["<@A_USER_ID>"]
```

사용자 ID를 얻으려면: 디스코드 **설정 → 고급 → 개발자 모드**를 켠 뒤, 사용자를
우클릭 → **사용자 ID 복사**. 여러 명을 호출하려면 여러 개를 나열하세요, 예:
`mentions: ["<@A>", "<@B>"]`.

## 개발

```bash
go test ./...            # 전체 테스트 실행
go test -cover ./...     # 커버리지 포함
go vet ./...
```

아키텍처 (설계 문서는 `docs/superpowers/specs/` 참고):

```
cmd/ninalertbot   진입점, 플래그, 시그널 처리
internal/config   config.yaml 로드 + 검증
internal/store    상품 페이지 조회 -> Available / SoldOut / Unknown
internal/notifier 디스코드 웹훅 클라이언트 + 메시지 포맷팅
internal/monitor  스케줄러, 상태 전환, 중복 제거
internal/state    마지막 상태 JSON 영속화
```
