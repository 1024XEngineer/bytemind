# 閰嶇疆鍙傝€?

`.bytemind/config.json` 鎵€鏈夊瓧娈电殑瀹屾暣璇存槑銆?

鍙敤绀轰緥鍙傝€?[`config.example.json`](https://github.com/1024XEngineer/bytemind/blob/main/config.example.json)銆?

## `provider`

妯″瀷 Provider 閰嶇疆銆?

| 瀛楁                | 绫诲瀷   | 璇存槑                                     | 榛樿鍊?                     |
| ------------------- | ------ | ---------------------------------------- | --------------------------- |
| `type`              | string | `openai-compatible`銆乣anthropic` 鎴?`gemini` | `openai-compatible`      |
| `base_url`          | string | API 绔偣 URL                             | `https://api.openai.com/v1` |
| `model`             | string | 浣跨敤鐨勬ā鍨?ID                            | `gpt-5.4-mini`              |
| `api_key`           | string | API 瀵嗛挜锛堟槑鏂囷紝寤鸿鏀圭敤 `api_key_env`锛?| 鈥?                          |
| `api_key_env`       | string | 浠庤鐜鍙橀噺璇诲彇 API 瀵嗛挜                | `BYTEMIND_API_KEY`          |
| `anthropic_version` | string | Anthropic API 鐗堟湰澶?                    | `2023-06-01`                |
| `auth_header`       | string | 鑷畾涔夐壌鏉冨ご鍚嶇О                         | `Authorization`             |
| `auth_scheme`       | string | 閴存潈鍓嶇紑锛堝 `Bearer`锛?                 | `Bearer`                    |
| `auto_detect_type`  | bool   | 鏍规嵁 `base_url` 鑷姩鎺ㄦ柇 Provider 绫诲瀷   | `false`                     |

## `approval_policy`

| 鍊?                  | 琛屼负                         |
| -------------------- | ---------------------------- |
| `on-request`锛堥粯璁わ級 | 姣忔楂橀闄╁伐鍏疯皟鐢ㄥ墠绛夊緟纭 |

## `approval_mode`

| 鍊?                   | 琛屼负                                  |
| --------------------- | ------------------------------------- |
| `interactive`锛堥粯璁わ級 | 浜や簰寮忓鎵癸紝姣忔鎿嶄綔寮瑰嚭纭          |
| `full_access`         | 鍏ㄩ儴鏉冮檺妯″紡锛屽鎵硅姹傝嚜鍔ㄩ€氳繃涓斾笉涓柇浠诲姟 |

鍏煎璇存槑锛氫负閬垮厤闈欓粯鎻愭潈锛宍approval_mode: away` 榛樿琚樆姝€備粎鍦ㄨ縼绉绘棫閰嶇疆鏃讹紝鏄惧紡璁剧疆 `BYTEMIND_ALLOW_AWAY_FULL_ACCESS=true` 鎵嶄細涓存椂鏄犲皠鍒?`full_access`銆?

## `away_policy`

宸插純鐢ㄥ吋瀹瑰瓧娈点€備繚鐣欑敤浜庡吋瀹规棫閰嶇疆褰㈢姸锛屼笉鍐嶅奖鍝嶈繍琛屾椂琛屼负銆?

| 鍊?                          | 琛屼负                         |
| ---------------------------- | ---------------------------- |
| `auto_deny_continue`锛堥粯璁わ級 | 浠呯敤浜庡吋瀹规棫閰嶇疆锛屼笉鍐嶅奖鍝嶈繍琛屾椂琛屼负 |
| `fail_fast`                  | 浠呯敤浜庡吋瀹规棫閰嶇疆锛屼笉鍐嶅奖鍝嶈繍琛屾椂琛屼负 |


## `notifications.desktop`

妗岄潰閫氱煡閰嶇疆銆?

| 瀛楁                    | 绫诲瀷 | 榛樿鍊?| 璇存槑 |
| ----------------------- | ---- | ------ | ---- |
| `enabled`               | bool | `true` | 妗岄潰閫氱煡鎬诲紑鍏炽€? |
| `on_approval_required`  | bool | `true` | 鍑虹幇瀹℃壒璇锋眰鏃跺彂閫侀€氱煡銆? |
| `on_run_completed`      | bool | `true` | 浠诲姟鎴愬姛瀹屾垚鏃跺彂閫侀€氱煡銆? |
| `on_run_failed`         | bool | `true` | 浠诲姟澶辫触鏃跺彂閫侀€氱煡銆? |
| `on_run_canceled`       | bool | `false` | 浠诲姟鍙栨秷鏃跺彂閫侀€氱煡銆? |
| `cooldown_seconds`      | int  | `3` | 鍚屼竴閫氱煡 key 鍐呯殑鍘婚噸鏃堕棿绐楀彛锛?`0` 琛ㄧず鍏抽棴 cooldown 鍘婚噸銆? |
## `max_iterations`

| 绫诲瀷    | 榛樿鍊?|
| ------- | ------ |
| integer | `32`   |

鍗曚换鍔℃渶澶у伐鍏疯皟鐢ㄨ疆娆°€傚埌杈句笂闄愬悗 Agent 杈撳嚭闃舵鎬ф€荤粨骞跺仠姝€?

## `stream`

| 绫诲瀷 | 榛樿鍊?|
| ---- | ------ |
| bool | `true` |

寮€鍚祦寮忚緭鍑恒€傞潪 TTY 鐜锛堝 CI 绠￠亾锛夊缓璁涓?`false`銆?

## `sandbox_enabled`

| 绫诲瀷 | 榛樿鍊? |
| ---- | ------- |
| bool | `false` |

璁句负 `true` 鍚庯紝鏂囦欢鍜?Shell 宸ュ叿鐨勫啓鍏ユ搷浣滃皢琚檺鍒跺湪 `writable_roots` 鑼冨洿鍐呫€?

## `writable_roots`

| 绫诲瀷     | 榛樿鍊?|
| -------- | ------ |
| string[] | `[]`   |

寮€鍚矙绠辨椂鍏佽鍐欏叆鐨勭洰褰曞垪琛ㄣ€?

## `exec_allowlist`

璺宠繃瀹℃壒鎻愮ず鐨?Shell 鍛戒护鐧藉悕鍗曘€?

```json
{
  "exec_allowlist": [
    { "command": "go", "args_pattern": ["test", "./..."] },
    { "command": "make", "args_pattern": ["build"] }
  ]
}
```

## `token_quota`

| 绫诲瀷    | 榛樿鍊?  |
| ------- | -------- |
| integer | `300000` |

鍗曚細璇?token 娑堣€楅璀﹂槇鍊笺€?

## `update_check`

| 瀛楁      | 绫诲瀷 | 榛樿鍊?| 璇存槑               |
| --------- | ---- | ------ | ------------------ |
| `enabled` | bool | `true` | 鍚姩鏃舵槸鍚︽鏌ユ洿鏂?|

## `context_budget`

涓婁笅鏂囩獥鍙ｇ敤閲忕鐞嗐€?

| 瀛楁                 | 绫诲瀷  | 榛樿鍊?| 璇存槑                           |
| -------------------- | ----- | ------ | ------------------------------ |
| `warning_ratio`      | float | `0.85` | 鐢ㄩ噺杈惧埌姝ゆ瘮渚嬫椂杈撳嚭璀﹀憡       |
| `critical_ratio`     | float | `0.95` | 鐢ㄩ噺杈惧埌姝ゆ瘮渚嬫椂瑙﹀彂鍘嬬缉鎴栧仠姝?|
| `max_reactive_retry` | int   | `1`    | 涓婁笅鏂囧帇缂╁悗鏈€澶ч噸璇曟鏁?      |

## 瀹屾暣绀轰緥

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-4o",
    "api_key_env": "OPENAI_API_KEY"
  },
  "approval_policy": "on-request",
  "approval_mode": "interactive",
  "notifications": {
    "desktop": {
      "enabled": true,
      "on_approval_required": true,
      "on_run_completed": true,
      "on_run_failed": true,
      "on_run_canceled": false,
      "cooldown_seconds": 3
    }
  },
  "max_iterations": 32,
  "stream": true,
  "sandbox_enabled": false,
  "writable_roots": [],
  "token_quota": 300000,
  "update_check": { "enabled": true },
  "context_budget": {
    "warning_ratio": 0.85,
    "critical_ratio": 0.95,
    "max_reactive_retry": 1
  }
}
```
