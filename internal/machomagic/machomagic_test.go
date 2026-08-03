//go:build test

package machomagic

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIs verifies that Is recognizes every supported Mach-O / Fat binary
// magic value in both byte orders and rejects other formats. (AC: スコープ(案)
// の整理結果 — isMachOMagic / isMachOMagicAll の差分(32bit除外)は実装上の揺れで
// あり、両者を統合した全マジック集合 {32/64-bit, Fat} × {native, swapped} を
// 判定すること)
func TestIs(t *testing.T) {
	tests := []struct {
		name string
		b    []byte
		want bool
	}{
		{
			name: "64-bit Mach-O (native endian)",
			b:    []byte("\xcf\xfa\xed\xfe"),
			want: true,
		},
		{
			name: "64-bit Mach-O (byte-swapped)",
			b:    []byte("\xfe\xed\xfa\xcf"),
			want: true,
		},
		{
			name: "32-bit Mach-O (native endian)",
			b:    []byte("\xce\xfa\xed\xfe"),
			want: true,
		},
		{
			name: "32-bit Mach-O (byte-swapped)",
			b:    []byte("\xfe\xed\xfa\xce"),
			want: true,
		},
		{
			name: "Fat binary (native endian)",
			b:    []byte("\xca\xfe\xba\xbe"),
			want: true,
		},
		{
			name: "Fat binary (byte-swapped)",
			b:    []byte("\xbe\xba\xfe\xca"),
			want: true,
		},
		{
			name: "ELF magic",
			b:    []byte("\x7fELF"),
			want: false,
		},
		{
			name: "PE magic (MZ)",
			b:    []byte("MZ\x90\x00"),
			want: false,
		},
		{
			name: "too short",
			b:    []byte("\xcf\xfa"),
			want: false,
		},
		{
			name: "empty",
			b:    []byte{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Is(tt.b))
		})
	}
}

// TestIsFat verifies that IsFat recognizes only Fat binary magic values
// (both byte orders) and rejects single-arch Mach-O magic. (AC: machodylib の
// looksLikeMachO が Fat を除外するのはコメント明記の意図的差分であるため、
// IsFat として分離し「Is && !IsFat」で単一アーキテクチャ判定を維持できること)
func TestIsFat(t *testing.T) {
	tests := []struct {
		name string
		b    []byte
		want bool
	}{
		{
			name: "Fat binary (native endian)",
			b:    []byte("\xca\xfe\xba\xbe"),
			want: true,
		},
		{
			name: "Fat binary (byte-swapped)",
			b:    []byte("\xbe\xba\xfe\xca"),
			want: true,
		},
		{
			name: "64-bit Mach-O",
			b:    []byte("\xcf\xfa\xed\xfe"),
			want: false,
		},
		{
			name: "32-bit Mach-O",
			b:    []byte("\xce\xfa\xed\xfe"),
			want: false,
		},
		{
			name: "too short",
			b:    []byte("\xca\xfe"),
			want: false,
		},
		{
			name: "empty",
			b:    []byte{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsFat(tt.b))
		})
	}
}
