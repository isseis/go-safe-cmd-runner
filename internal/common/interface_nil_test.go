package common

import "testing"

func TestIsNilInterfaceValue(t *testing.T) {
	t.Parallel()

	t.Run("untyped nil", func(t *testing.T) {
		if !IsNilInterfaceValue(nil) {
			t.Fatal("expected nil to be treated as nil")
		}
	})

	t.Run("typed nil pointer", func(t *testing.T) {
		type sample struct{}
		var p *sample
		if !IsNilInterfaceValue(p) {
			t.Fatal("expected typed nil pointer to be treated as nil")
		}
	})

	t.Run("typed nil map", func(t *testing.T) {
		var m map[string]string
		if !IsNilInterfaceValue(m) {
			t.Fatal("expected typed nil map to be treated as nil")
		}
	})

	t.Run("typed nil slice", func(t *testing.T) {
		var s []string
		if !IsNilInterfaceValue(s) {
			t.Fatal("expected typed nil slice to be treated as nil")
		}
	})

	t.Run("typed nil channel", func(t *testing.T) {
		var ch chan int
		if !IsNilInterfaceValue(ch) {
			t.Fatal("expected typed nil channel to be treated as nil")
		}
	})

	t.Run("typed nil func", func(t *testing.T) {
		var fn func()
		if !IsNilInterfaceValue(fn) {
			t.Fatal("expected typed nil func to be treated as nil")
		}
	})

	t.Run("non-nil pointer", func(t *testing.T) {
		type sample struct{}
		v := &sample{}
		if IsNilInterfaceValue(v) {
			t.Fatal("expected non-nil pointer to be treated as non-nil")
		}
	})

	t.Run("non-nil map", func(t *testing.T) {
		m := map[string]string{"k": "v"}
		if IsNilInterfaceValue(m) {
			t.Fatal("expected non-nil map to be treated as non-nil")
		}
	})

	t.Run("non-nil concrete value", func(t *testing.T) {
		if IsNilInterfaceValue(0) {
			t.Fatal("expected concrete non-nil value to be treated as non-nil")
		}
	})
}
