package util

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAddFinalizer_EmptyInitial(t *testing.T) {
	m := &metav1.ObjectMeta{
		Finalizers: []string{},
	}
	b := AddFinalizer(m, "foo")
	if b == false {
		t.Errorf("AddFinalizer returned false when adding a new finalizer")
	}
	if !reflect.DeepEqual(m.Finalizers, []string{"foo"}) {
		t.Errorf("AddFinalizer did not add a finalizer")
	}
}

func TestAddFinalizer_ExistingInitial(t *testing.T) {
	m := &metav1.ObjectMeta{
		Finalizers: []string{"foo", "bar", "baz"},
	}
	b := AddFinalizer(m, "foo")
	if b == true {
		t.Errorf("AddFinalizer returned true when adding an existing finalizer")
	}
	if !reflect.DeepEqual(m.Finalizers, []string{"foo", "bar", "baz"}) {
		t.Errorf("AddFinalizer changed a finalizer when it shouldn't")
	}
}

func TestRemoveFinalizer_EmptyInitial(t *testing.T) {
	m := &metav1.ObjectMeta{
		Finalizers: []string{},
	}
	b := RemoveFinalizer(m, "foo")
	if b == true {
		t.Errorf("Removed finalizer that wasn't present")
	}
	if !reflect.DeepEqual(m.Finalizers, []string{}) {
		t.Errorf("Remove changed a finalizer when it shouldn't")
	}
}

func TestRemoveFinalizer_ExistingInitial(t *testing.T) {
	m := &metav1.ObjectMeta{
		Finalizers: []string{"foo", "bar", "baz"},
	}
	b := RemoveFinalizer(m, "foo")
	if b == false {
		t.Errorf("RemoveFinalizer failed")
	}
	if !reflect.DeepEqual(m.Finalizers, []string{"bar", "baz"}) {
		t.Errorf("RemoveFinalizer didn't")
	}
}
