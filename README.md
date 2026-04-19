<p align="center">
  <img alt="ByteMind Banner" src="https://capsule-render.vercel.app/api?type=waving&height=190&color=0:0ea5e9,100:2563eb&text=ByteMind&fontColor=ffffff&fontSize=52&animation=fadeIn" />
</p>

<h1 align="center">ByteMind</h1>

<p align="center">
  <a href="https://github.com/1024XEngineer/bytemind/stargazers"><img alt="GitHub Stars" src="https://img.shields.io/github/stars/1024XEngineer/bytemind?style=for-the-badge&logo=github&color=f59e0b" /></a>
  <a href="https://github.com/1024XEngineer/bytemind/network/members"><img alt="GitHub Forks" src="https://img.shields.io/github/forks/1024XEngineer/bytemind?style=for-the-badge&logo=github&color=06b6d4" /></a>
  <a href="https://github.com/1024XEngineer/bytemind/blob/main/LICENSE"><img alt="License" src="https://img.shields.io/badge/License-MIT-22c55e?style=for-the-badge" /></a>
  <img alt="Go Version" src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=for-the-badge&logo=go&logoColor=white" />
  <a href="https://github.com/1024XEngineer/bytemind/commits/main"><img alt="Last Commit" src="https://img.shields.io/github/last-commit/1024XEngineer/bytemind/main?style=for-the-badge&color=8b5cf6" /></a>
</p>

<p align="center">
  涓€涓敤 Go 瀹炵幇鐨?AI Coding CLI锛岀洰鏍囨槸鎻愪緵鏇存帴杩?OpenCode / ClaudeCode 鐨勫伐浣滄祦鑳藉姏銆?
</p>

<p align="center">
  <a href="#core-features">鏍稿績鑳藉姏</a> 鈥?
  <a href="#quick-start">蹇€熷紑濮?/a> 鈥?
  <a href="#configuration">閰嶇疆鏂囦欢</a> 鈥?
  <a href="#project-structure">鐩綍缁撴瀯</a>
</p>

> [!NOTE]
> 褰撳墠鐗堟湰宸插叿澶囧杞細璇濄€佹祦寮忚緭鍑恒€佸伐鍏疯皟鐢ㄥ惊鐜€丼hell 鎵ц瀹℃壒銆佹墽琛岄绠楁帶鍒朵笌閲嶅璋冪敤妫€娴嬬瓑鏍稿績鑳藉姏銆?

> [!TIP]
> 闀夸换鍔″缓璁彁楂?`-max-iterations`锛屽埌杈鹃绠楀悗浼氳繑鍥為樁娈垫€ф€荤粨锛屼笉浼氱洿鎺ュけ璐ラ€€鍑恒€?

<a id="core-features"></a>

## 馃幆 鏍稿績鑳藉姏

| 妯″潡 | 璇存槑 | 鐘舵€?|
| --- | --- | --- |
| 浼氳瘽绯荤粺 | 澶氳疆浼氳瘽 + 浼氳瘽鎸佷箙鍖?| ![status](https://img.shields.io/badge/status-ready-22c55e?style=flat-square) |
| 瀵硅瘽浜や簰 | 绾?CLI 鑱婂ぉ + 娴佸紡缁堢杈撳嚭 | ![status](https://img.shields.io/badge/status-ready-22c55e?style=flat-square) |
| Provider 閫傞厤 | OpenAI-compatible / Anthropic 鍙岄€傞厤灞?| ![status](https://img.shields.io/badge/status-ready-22c55e?style=flat-square) |
| 宸ュ叿鎵ц | 鏂囦欢璇诲啓鎼滅储銆佽ˉ涓佹浛鎹€佸懡浠ゆ墽琛屽鎵?| ![status](https://img.shields.io/badge/status-ready-22c55e?style=flat-square) |
| 杩愯绋冲畾鎬?| `max_iterations` 棰勭畻鎺у埗 + 閲嶅璋冪敤妫€娴?| ![status](https://img.shields.io/badge/status-ready-22c55e?style=flat-square) |

<a id="quick-start"></a>

## 馃殌 蹇€熷紑濮?

## 瀹夎锛堟棤闇€ Go锛?

### 0) 涓€閿畨瑁?

macOS / Linux锛?

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash
```

Windows PowerShell锛?

```powershell
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

瀹夎鎸囧畾鐗堟湰锛堢ず渚?`v0.3.0`锛夛細

```bash
BYTEMIND_VERSION=v0.3.0 curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash
```

```powershell
$env:BYTEMIND_VERSION='v0.3.0'; iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

鏇村瀹夎鏂瑰紡瑙侊細[docs/installation.md](docs/installation.md)

### 1) 鍑嗗閰嶇疆

鍏堝鍒剁ず渚嬮厤缃紝鍐嶆妸 `api_key` 绛夊瓧娈垫敼鎴愪綘鑷繁鐨勫€硷細

```powershell
New-Item -ItemType Directory -Force .bytemind | Out-Null
Copy-Item config.example.json .bytemind/config.json
```

> 鍏煎璇存槑锛氬伐浣滃尯 `config.json` 涔熶細琚瘑鍒紱杩欓噷鎺ㄨ崘 `.bytemind/config.json` 鏂逛究涓庢簮鐮佸垎绂汇€?

### 2) 杩愯 ByteMind

鑱婂ぉ妯″紡锛?

```powershell
go run ./cmd/bytemind chat
```

鍗曟浠诲姟锛?

```powershell
go run ./cmd/bytemind run -prompt "鍒嗘瀽褰撳墠椤圭洰骞剁敓鎴愭敼杩涘缓璁?
```

鎻愰珮鎵ц棰勭畻锛?

```powershell
go run ./cmd/bytemind chat -max-iterations 64
go run ./cmd/bytemind run -prompt "refactor this module" -max-iterations 64
```

<a id="configuration"></a>

## 鈿欙笍 閰嶇疆鏂囦欢

榛樿閰嶇疆锛圤penAI-compatible锛夛細

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4-mini",
    "api_key": "your-api-key-here"
  },
  "approval_policy": "on-request",
  "max_iterations": 32,
  "stream": true
}
```

<details>
<summary>Anthropic 閰嶇疆绀轰緥</summary>

```json
{
  "provider": {
    "type": "anthropic",
    "base_url": "https://api.anthropic.com",
    "model": "claude-sonnet-4-20250514",
    "api_key": "your-api-key-here",
    "anthropic_version": "2023-06-01"
  }
}
```

</details>

<a id="project-structure"></a>

## 馃П 鐩綍缁撴瀯

```text
cmd/bytemind            CLI 鍏ュ彛
internal/agent          瀵硅瘽寰幆銆佺郴缁熸彁绀鸿瘝妯℃澘銆佹祦寮忚緭鍑?
internal/config         閰嶇疆鍔犺浇涓庣幆澧冨彉閲忚鐩?
internal/llm            閫氱敤娑堟伅涓庡伐鍏风被鍨?
internal/provider       澶?provider 閫傞厤灞?
internal/session        浼氳瘽鎸佷箙鍖?
internal/tools          鏂囦欢宸ュ叿銆乸atch 宸ュ叿銆乻hell 宸ュ叿
```

## 馃Л 浜や簰鍛戒护

- `/help`
- `/session`
- `/sessions`
- `/quit`

## 馃О 宸插疄鐜板伐鍏?

- `list_files`
- `read_file`
- `search_text`
- `write_file`
- `replace_in_file`
- `apply_patch`
- `run_shell`

## 馃摑 绯荤粺鎻愮ず璇嶇淮鎶?

绯荤粺鎻愮ず璇嶆ā鏉垮湪锛?

- `internal/agent/prompts/system.md`

杩愯鏃剁敱 `internal/agent/prompt.go` 閫氳繃 `go:embed` 鍐呭祵 Markdown锛屽苟鏇挎崲 `{{WORKSPACE}}`銆乣{{APPROVAL_POLICY}}` 鍗犱綅绗︺€?

## 馃實 Environment Variables

See [docs/environment-variables.md](docs/environment-variables.md) for runtime TUI env vars:

- `BYTEMIND_ENABLE_MOUSE`
- `BYTEMIND_WINDOWS_INPUT_TTY`
- `BYTEMIND_MOUSE_Y_OFFSET`

## 馃搫 License

MIT License
