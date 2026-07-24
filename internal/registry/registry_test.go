package registry

import (
	"reflect"
	"testing"
)

func TestRegisterAndGet(t *testing.T) {
	r := New[int]("test", "thing")
	r.Register("b", 2)
	r.Register("a", 1)

	if v, ok := r.Get("a"); !ok || v != 1 {
		t.Errorf("Get(a) = %d, %t; want 1, true", v, ok)
	}
	if v, ok := r.Get("b"); !ok || v != 2 {
		t.Errorf("Get(b) = %d, %t; want 2, true", v, ok)
	}
	if _, ok := r.Get("missing"); ok {
		t.Error("Get(missing) reported ok for an unregistered name")
	}
}

func TestNamesSorted(t *testing.T) {
	r := New[string]("test", "thing")
	r.Register("zebra", "z")
	r.Register("apple", "a")
	r.Register("mango", "m")

	got := r.Names()
	want := []string{"apple", "mango", "zebra"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Names() = %v; want %v", got, want)
	}
}

func TestValuesSortedNameOrder(t *testing.T) {
	r := New[string]("test", "thing")
	r.Register("zebra", "z")
	r.Register("apple", "a")
	r.Register("mango", "m")

	got := r.Values()
	want := []string{"a", "m", "z"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Values() = %v; want %v", got, want)
	}
}

func TestDuplicatePanicsWithLabel(t *testing.T) {
	r := New[int]("forge", "adapter")
	r.Register("github", 1)

	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatal("duplicate Register did not panic")
		}
		msg, ok := rec.(string)
		if !ok {
			t.Fatalf("panic value = %v (%T); want string", rec, rec)
		}
		want := "forge: duplicate adapter github"
		if msg != want {
			t.Errorf("panic message = %q; want %q", msg, want)
		}
	}()
	r.Register("github", 2)
}
