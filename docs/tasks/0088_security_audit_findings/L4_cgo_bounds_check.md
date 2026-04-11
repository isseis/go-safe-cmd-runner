# L4: CGO 側 count 値の Go 側境界チェック欠如

- **重大度**: 🟠 Low
- **領域**: CGO (`internal/groupmembership`)
- **影響コマンド**: `record`, `verify`, `runner`

## 問題

[internal/groupmembership/membership_cgo.go:106](../../../internal/groupmembership/membership_cgo.go#L106) 付近で C 側から受け取った `count` と `members` ポインタを使って Go slice を構築している:

```go
cArray := (*[1 << 30]*C.char)(unsafe.Pointer(members))[:count:count]
```

- `count` は C 側 (`getgrgid_r` ラッパ) から返される `gr_mem` の要素数。
- この値に対する Go 側での **上限チェック** (`count >= 0 && count < reasonableMax`) が無い。
- `(*[1 << 30]*C.char)` キャストは事実上無制限のインデックスを許す。

## 影響

### 直接的な攻撃可能性は低い

- `count` の出所は glibc の `getgrgid_r` 結果であり、攻撃者が直接制御できる入力ではない。
- `getgrgid_r` は `/etc/group` (または NSS バックエンド) をパースするため、そこに異常に長いグループメンバリストがあれば理論上は大きな値を返しうる。
- `/etc/group` を書き換えられる攻撃者はそもそも root 権限を持つため、このコード経路を攻める必然性は薄い。

### 潜在的な問題

- C 側のバグや NSS プラグインの異常により `count` が負値 (int として渡された場合) や極端に大きい値を返した場合、Go 側で panic せず不正メモリにアクセスする可能性がある。
- 防御的コーディングの観点で、`unsafe.Pointer` からの slice 構築には **必ず** 境界チェックを付けるのが定石。

## 修正方針

### 案 A (推奨): 境界チェックの追加

```go
const maxGroupMembers = 65536 // 現実的上限

if count < 0 || count > maxGroupMembers {
    return nil, fmt.Errorf("unexpected group member count: %d", count)
}
cArray := (*[1 << 30]*C.char)(unsafe.Pointer(members))[:count:count]
```

### 案 B: `unsafe.Slice` への移行

Go 1.17+ の `unsafe.Slice` は境界チェック付きで slice を構築する:

```go
cArray := unsafe.Slice((**C.char)(unsafe.Pointer(members)), count)
```

- 依然として `count` の健全性は呼び出し側で担保する必要があるが、巨大配列キャストパターンを排除できる。
- 本プロジェクトは Go 1.23.10 を使っており利用可能。

### 推奨

**案 A + 案 B の併用**。まず `count` の上限をチェックし、その後 `unsafe.Slice` で slice を構築する。

## 参考箇所

- [internal/groupmembership/membership_cgo.go:106](../../../internal/groupmembership/membership_cgo.go#L106) — slice 構築箇所
- Go 公式: `unsafe.Slice` (Go 1.17+)
