package util

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AddFinalizer adds the specified finalizer to the meta, if not already present
// returns true if a change was made, and false if there were no changes
// This allows the calling code to decide whether to update the object
func AddFinalizer(meta *metav1.ObjectMeta, finalizer string) bool {
	for _, v := range meta.Finalizers {
		if v == finalizer {
			return false
		}
	}
	meta.SetFinalizers(append(meta.Finalizers, finalizer))
	return true
}

// RemoveFinalizer removes the specified finalizer from the meta, if present
// returns true if a change was made, and false if there were no changes
// This allows the calling code to decide whether to update the object
func RemoveFinalizer(meta *metav1.ObjectMeta, finalizer string) bool {
	for i, v := range meta.Finalizers {
		if v == finalizer {
			meta.SetFinalizers(append(meta.Finalizers[:i], meta.Finalizers[i+1:]...))
			return true
		}
	}
	return false
}
